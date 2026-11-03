package candidatefeedback

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"hr-assistant/openrouter-worker/internal/app/actions"
	feedback "hr-assistant/shared/candidatefeedback"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFeedbackHandlerValidatesAndRetriesSeparately(t *testing.T) {
	answer := "Я увеличил продажи на 20%."
	job := actions.Job{Model: "test-model", PromptVersion: feedback.PromptVersion, SchemaVersion: feedback.SchemaVersion,
		FeedbackInput: feedback.Input{Summary: "Описан рост продаж.", Answers: []feedback.Answer{{QuestionNumber: 1, Question: "Каков результат?", Text: &answer}}}}
	responseText := `{"paragraphs":["Вы привели измеримый результат — рост продаж на 20%.","В следующий раз подробнее раскройте ваши действия."]}`
	client := &http.Client{Transport: roundTrip(func(request *http.Request) (*http.Response, error) {
		var payload struct {
			Messages       []struct{ Role, Content string }
			ResponseFormat map[string]any `json:"response_format"`
		}
		require.NoError(t, json.NewDecoder(request.Body).Decode(&payload))
		require.Equal(t, "system", payload.Messages[0].Role)
		require.Contains(t, payload.Messages[0].Content, "не более трёх")
		require.Contains(t, payload.Messages[1].Content, answer)
		require.NotContains(t, payload.Messages[0].Content, "untrusted error")
		require.Equal(t, "json_schema", payload.ResponseFormat["type"])
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": responseText}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})}
	handler := NewHandler(client, "https://provider.test", "test-key")
	result, failure := handler.Execute(context.Background(), job)
	require.Nil(t, failure)
	require.NotEmpty(t, result.Feedback)
	require.Empty(t, result.Report)
	require.Empty(t, result.Evaluation)
	responseText = `{"paragraphs":["one","two","three","four"]}`
	for attempt := 0; attempt < 3; attempt++ {
		job.InvalidAttempts = attempt
		job.LastErrorMessage = "untrusted error"
		result, failure = handler.Execute(context.Background(), job)
		require.Empty(t, result.Feedback)
		require.NotNil(t, failure)
		require.Equal(t, "invalid_result", failure.Code)
		require.Equal(t, attempt == 2, failure.Permanent)
	}
}
