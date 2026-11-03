package candidatefeedback

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"hr-assistant/openrouter-worker/internal/app/actions"
	"hr-assistant/openrouter-worker/internal/app/structured"
	"hr-assistant/openrouter-worker/prompts"
	feedback "hr-assistant/shared/candidatefeedback"
)

type Handler struct{ client *structured.Client }

func NewHandler(client *http.Client, baseURL, apiKey string) *Handler {
	return &Handler{structured.NewClient(client, baseURL, apiKey)}
}

func (*Handler) Type() string { return feedback.Action }

func (h *Handler) Execute(ctx context.Context, job actions.Job) (actions.Result, *actions.Failure) {
	if job.PromptVersion != feedback.PromptVersion || job.SchemaVersion != feedback.SchemaVersion {
		return actions.Result{}, structured.Failure(true, "unsupported_version")
	}
	if !job.FeedbackInput.Valid() || strings.TrimSpace(job.Model) == "" {
		return actions.Result{}, structured.Failure(true, "invalid_input")
	}
	result, failure := h.client.Execute(ctx, structured.Request{Model: job.Model, Prompt: prompts.CandidateFeedbackV1,
		Name: feedback.Action, Schema: feedback.Schema, Input: job.FeedbackInput,
		InvalidAttempts: job.InvalidAttempts, LastErrorMessage: job.LastErrorMessage,
		Validate: func(raw []byte) (json.RawMessage, error) {
			parsed, err := feedback.Parse(raw)
			if err != nil {
				return nil, err
			}
			return json.Marshal(parsed)
		}})
	result.Feedback, result.Evaluation = result.Evaluation, nil
	return result, failure
}
