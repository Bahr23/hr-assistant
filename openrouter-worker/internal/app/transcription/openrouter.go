package transcription

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"hr-assistant/openrouter-worker/internal/app/actions"
)

type OpenRouterClient struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

func NewOpenRouterClient(httpClient *http.Client, baseURL, apiKey string) *OpenRouterClient {
	return &OpenRouterClient{
		httpClient: httpClient,
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
	}
}

type transcriptionRequest struct {
	Model       string     `json:"model"`
	InputAudio  inputAudio `json:"input_audio"`
	Language    string     `json:"language,omitempty"`
	Temperature float64    `json:"temperature"`
}

type inputAudio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type transcriptionResponse struct {
	Text  string `json:"text"`
	Model string `json:"model"`
	Usage struct {
		Seconds          *float64 `json:"seconds"`
		TotalTokens      *int64   `json:"total_tokens"`
		PromptTokens     *int64   `json:"prompt_tokens"`
		CompletionTokens *int64   `json:"completion_tokens"`
		InputTokens      *int64   `json:"input_tokens"`
		OutputTokens     *int64   `json:"output_tokens"`
		Cost             *float64 `json:"cost"`
	} `json:"usage"`
}

func (c *OpenRouterClient) Transcribe(
	ctx context.Context,
	path, format, model, language string,
) (actions.Result, *actions.Failure) {
	audio, err := os.ReadFile(path)
	if err != nil {
		return actions.Result{}, transient("temp_file_read_failed", "could not read temporary audio file")
	}
	if len(audio) == 0 {
		return actions.Result{}, permanent("empty_audio", "audio file is empty")
	}
	if int64(len(audio)) > MaxAudioBytes {
		return actions.Result{}, permanent("audio_too_large", "audio exceeds the 20 MB limit")
	}
	requestPayload := transcriptionRequest{
		Model:       model,
		InputAudio:  inputAudio{Data: base64.StdEncoding.EncodeToString(audio), Format: format},
		Language:    normalizeLanguage(language),
		Temperature: 0,
	}
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return actions.Result{}, permanent("request_encode_failed", "could not encode transcription request")
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/audio/transcriptions", bytes.NewReader(body),
	)
	if err != nil {
		return actions.Result{}, permanent("request_build_failed", "could not build transcription request")
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return actions.Result{}, transient("openrouter_unavailable", "OpenRouter is temporarily unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
		switch response.StatusCode {
		case http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
			return actions.Result{}, permanent("unsupported_audio", "OpenRouter rejected the audio file")
		case http.StatusRequestTimeout, http.StatusTooManyRequests:
			return actions.Result{}, transient("openrouter_rate_limited", "OpenRouter request should be retried")
		default:
			return actions.Result{}, transient("openrouter_unavailable", "OpenRouter is temporarily unavailable")
		}
	}
	var decoded transcriptionResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	if err := decoder.Decode(&decoded); err != nil {
		return actions.Result{}, transient("openrouter_response_invalid", "OpenRouter returned an invalid response")
	}
	if strings.TrimSpace(decoded.Text) == "" {
		return actions.Result{}, transient("empty_transcript", "OpenRouter returned an empty transcript")
	}
	inputTokens := decoded.Usage.InputTokens
	if inputTokens == nil {
		inputTokens = decoded.Usage.PromptTokens
	}
	outputTokens := decoded.Usage.OutputTokens
	if outputTokens == nil {
		outputTokens = decoded.Usage.CompletionTokens
	}
	resultModel := strings.TrimSpace(decoded.Model)
	if resultModel == "" {
		resultModel = model
	}
	return actions.Result{
		Text: strings.TrimSpace(decoded.Text), Model: resultModel,
		GenerationID: response.Header.Get("X-Generation-Id"),
		Usage: actions.Usage{
			Seconds: decoded.Usage.Seconds, TotalTokens: decoded.Usage.TotalTokens,
			InputTokens: inputTokens, OutputTokens: outputTokens, Cost: decoded.Usage.Cost,
		},
	}, nil
}

func normalizeLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) >= 2 {
		return value[:2]
	}
	return value
}
