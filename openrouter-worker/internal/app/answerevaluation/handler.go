package answerevaluation

import (
	"context"
	"encoding/json"
	"hr-assistant/openrouter-worker/internal/app/actions"
	"hr-assistant/openrouter-worker/internal/app/structured"
	"hr-assistant/openrouter-worker/prompts"
	"hr-assistant/shared/evaluation"
	"net/http"
	"strings"
)

type Handler struct{ client *structured.Client }

func NewHandler(client *http.Client, baseURL, apiKey string) *Handler {
	return &Handler{structured.NewClient(client, baseURL, apiKey)}
}
func (*Handler) Type() string { return evaluation.Action }
func (h *Handler) Execute(ctx context.Context, job actions.Job) (actions.Result, *actions.Failure) {
	if job.PromptVersion != evaluation.PromptVersion || job.SchemaVersion != evaluation.SchemaVersion {
		return actions.Result{}, structured.Failure(true, "unsupported_version")
	}
	if !job.EvaluationInput.Valid() || strings.TrimSpace(job.Model) == "" {
		return actions.Result{}, structured.Failure(true, "invalid_input")
	}
	return h.client.Execute(ctx, structured.Request{Model: job.Model, Prompt: prompts.AnswerEvaluationV1, Name: evaluation.Action, Schema: evaluation.Schema, Input: job.EvaluationInput, InvalidAttempts: job.InvalidAttempts, LastErrorMessage: job.LastErrorMessage, Validate: func(raw []byte) (json.RawMessage, error) {
		raw = structured.RestoreAnswerEvidenceCase(raw, job.EvaluationInput.Answer)
		result, err := evaluation.Parse(raw, job.EvaluationInput.Answer)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	}})
}
