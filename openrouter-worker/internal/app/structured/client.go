package structured

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hr-assistant/openrouter-worker/internal/app/actions"
	feedback "hr-assistant/shared/candidatefeedback"
	report "hr-assistant/shared/candidatereport"
)

type Client struct {
	client          *http.Client
	baseURL, apiKey string
}

func NewClient(client *http.Client, baseURL, apiKey string) *Client {
	return &Client{client: client, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey}
}

type Request struct {
	Model, Prompt, Name string
	Schema              json.RawMessage
	Input               any
	InvalidAttempts     int
	LastErrorMessage    string
	Validate            func([]byte) (json.RawMessage, error)
}

func (h *Client) Execute(ctx context.Context, job Request) (actions.Result, *actions.Failure) {
	input, err := json.Marshal(job.Input)
	if err != nil {
		return actions.Result{}, Failure(true, "invalid_input")
	}
	prompt := job.Prompt
	if job.InvalidAttempts > 0 {
		prompt += "\n\n" + repairPrompt(job.LastErrorMessage, job.Name)
	}
	payload := map[string]any{
		"model": job.Model,
		"messages": []map[string]string{
			{"role": "system", "content": prompt},
			{"role": "user", "content": string(input)},
		},
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{
			"name": job.Name, "strict": true, "schema": job.Schema,
		}},
		"provider": map[string]any{"require_parameters": true},
	}
	// Terra providers do not accept temperature. Requiring an unsupported
	// parameter would prevent OpenRouter from selecting a compatible provider.
	if job.Model != "openai/gpt-5.6-terra" {
		payload["temperature"] = 0
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return actions.Result{}, Failure(true, "invalid_input")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return actions.Result{}, Failure(true, "invalid_input")
	}
	request.Header.Set("Authorization", "Bearer "+h.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := h.client.Do(request)
	if err != nil {
		return actions.Result{}, Failure(false, "openrouter_unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Never include provider response bodies or transport errors in logs/events.
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		if response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode >= 500 {
			f := Failure(false, "openrouter_unavailable")
			if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
				f.RetryAfter = time.Duration(min(seconds, 86400)) * time.Second
			} else if date, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
				f.RetryAfter = max(0, time.Until(date))
			}
			return actions.Result{}, f
		}
		if response.StatusCode == 401 || response.StatusCode == 402 || response.StatusCode == 403 {
			return actions.Result{}, Failure(true, "openrouter_auth_failed")
		}
		return actions.Result{}, Failure(true, "openrouter_rejected")
	}
	var decoded struct {
		Model string `json:"model"`
		ID    string `json:"id"`
		Usage struct {
			Input  *int64   `json:"prompt_tokens"`
			Output *int64   `json:"completion_tokens"`
			Total  *int64   `json:"total_tokens"`
			Cost   *float64 `json:"cost"`
		} `json:"usage"`
		Choices []struct {
			Message struct {
				Content string  `json:"content"`
				Refusal *string `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return actions.Result{}, Failure(false, "openrouter_unavailable")
	}
	if len(data) > 1<<20 || json.Unmarshal(data, &decoded) != nil {
		return actions.Result{}, invalidResult(job.InvalidAttempts, "invalid_response")
	}
	if decoded.Error != nil {
		return actions.Result{}, Failure(false, "openrouter_unavailable")
	}
	if len(decoded.Choices) != 1 {
		return actions.Result{}, invalidResult(job.InvalidAttempts, "invalid_choices")
	}
	choice := decoded.Choices[0]
	if choice.Message.Refusal != nil || choice.FinishReason == "content_filter" {
		return actions.Result{}, Failure(true, "model_refusal")
	}
	if choice.FinishReason != "stop" {
		return actions.Result{}, invalidResult(job.InvalidAttempts, "incomplete_response")
	}
	result, err := job.Validate([]byte(choice.Message.Content))
	if err != nil {
		reason := "validation_failed"
		var validation interface{ ValidationCode() string }
		if errors.As(err, &validation) {
			if _, ok := repairReasons[validation.ValidationCode()]; ok {
				reason = validation.ValidationCode()
			}
		}
		failure := invalidResult(job.InvalidAttempts, reason)
		var located interface{ ValidationPath() string }
		if job.Name == report.Action && errors.As(err, &located) && report.ValidResultPath(located.ValidationPath()) {
			failure.Message += ":" + located.ValidationPath()
		}
		return actions.Result{}, failure
	}
	model := strings.TrimSpace(decoded.Model)
	if model == "" {
		model = job.Model
	}
	return actions.Result{Evaluation: result, Model: model, GenerationID: decoded.ID, Usage: actions.Usage{InputTokens: decoded.Usage.Input, OutputTokens: decoded.Usage.Output, TotalTokens: decoded.Usage.Total, Cost: decoded.Usage.Cost}}, nil
}

func Failure(permanent bool, code string) *actions.Failure {
	return &actions.Failure{Permanent: permanent, Code: code, Message: code}
}

func invalidResult(invalidAttempts int, reason string) *actions.Failure {
	failure := Failure(invalidAttempts+1 >= 3, "invalid_result")
	failure.Message += ":" + reason
	return failure
}

// Only these developer-owned hints can enter a system message. Never interpolate
// raw validation errors, previous model output, or persisted arbitrary text.
var repairReasons = map[string]string{
	"result_too_large":        "Результат превысил допустимый размер. Сформулируй его короче.",
	"invalid_json":            "Результат не являлся JSON. Верни только один JSON-объект без Markdown.",
	"schema_mismatch":         "Результат не соответствует JSON Schema. Проверь обязательные поля, типы, статусы и ограничения массивов.",
	"empty_text":              "В результате были пустые строки вместо содержательных пояснений.",
	"missing_evidence":        "Для найденного признака отсутствовала обязательная цитата.",
	"unexpected_evidence":     "Для статуса not_found массив evidence должен быть пустым, даже если ответ прямо отрицает нежелательное поведение. Объясни отрицание в explanation.",
	"evidence_mismatch":       "Одна из цитат не является дословным фрагментом исходного ответа.",
	"criteria_count_mismatch": "Количество элементов criterion_scores должно совпадать с количеством входных criteria.",
	"duplicate_criterion":     "Один criterion_id повторён. Оцени каждый входной критерий ровно один раз.",
	"criteria_mismatch":       "Набор criterion_id не совпадает с входными criteria. Используй все исходные ID ровно по одному разу.",
	"criterion_name_mismatch": "Название name должно точно совпадать с title входного критерия с тем же ID.",
	"missing_score":           "Для status=scored нужен числовой score от 0 до 10. При нехватке данных используй insufficient_data, score=null и непустой missing_information.",
	"unexpected_score":        "Для status=insufficient_data поле score должно быть null; отсутствие данных не равно нулевому баллу.",
	"missing_information":     "Для status=insufficient_data нужен непустой missing_information с объяснением, каких данных не хватает.",
	"invalid_question_number": "Цитата ссылается на несуществующий ответ или ответ без текста. Используй question_number исходного ответа с доступным text, начиная с 1.",
}

func repairPrompt(previous, action string) string {
	if action == feedback.Action {
		return "Предыдущая попытка не прошла проверку. Верни только JSON с массивом paragraphs из одного-трёх непустых абзацев, каждый до 600 символов, без переносов строк и управляющих символов внутри элементов. Соблюдай исходные правила обратной связи и опирайся только на ответы кандидата."
	}
	prompt := "Предыдущая попытка не прошла проверку результата. Сформируй результат заново по тем же исходным данным, сохраняя все правила оценки. "
	diagnostic, valid := strings.CutPrefix(previous, "invalid_result:")
	code, path, hasPath := strings.Cut(diagnostic, ":")
	if hint, ok := repairReasons[code]; valid && ok {
		if action == report.Action && code == "missing_evidence" {
			hint = "Для status=scored нужна хотя бы одна дословная цитата в evidence этого критерия, включая score=0. Цитата из concerns или другого критерия не заменяет его evidence. Если данных недостаточно, используй status=insufficient_data, score=null и непустой missing_information. Каждая сильная сторона также требует своей цитаты."
		}
		if action == report.Action && hasPath && report.ValidResultPath(path) {
			prompt += "Ошибка " + code + " в поле " + path + ". Индексы массивов в пути начинаются с 0. "
		}
		prompt += hint + " "
	}
	return prompt + "Проверь JSON Schema и согласованность статусов с evidence. Цитаты копируй как непрерывные дословные фрагменты исходного текста: сохраняй регистр, пунктуацию и кавычки, не добавляй многоточия и не объединяй разные фрагменты. Не меняй выводы и не придумывай подтверждения ради прохождения проверки."
}
