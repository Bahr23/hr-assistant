package candidatereport

import (
	"context"
	"encoding/json"
	"hr-assistant/openrouter-worker/internal/app/actions"
	"hr-assistant/openrouter-worker/internal/app/structured"
	"hr-assistant/openrouter-worker/prompts"
	report "hr-assistant/shared/candidatereport"
	"net/http"
	"strings"
)

type Handler struct{ client *structured.Client }

func NewHandler(client *http.Client, baseURL, apiKey string) *Handler {
	return &Handler{structured.NewClient(client, baseURL, apiKey)}
}
func (*Handler) Type() string { return report.Action }
func (h *Handler) Execute(ctx context.Context, job actions.Job) (actions.Result, *actions.Failure) {
	if job.PromptVersion != report.PromptVersion || job.SchemaVersion != report.SchemaVersion {
		return actions.Result{}, structured.Failure(true, "unsupported_version")
	}
	if !job.ReportInput.Valid() || strings.TrimSpace(job.Model) == "" {
		return actions.Result{}, structured.Failure(true, "invalid_input")
	}
	result, failure := h.client.Execute(ctx, structured.Request{Model: job.Model, Prompt: prompts.CandidateReportV1, Name: report.Action, Schema: report.Schema, Input: job.ReportInput, InvalidAttempts: job.InvalidAttempts, LastErrorMessage: job.LastErrorMessage, Validate: func(raw []byte) (json.RawMessage, error) {
		answers := make(map[int]string, len(job.ReportInput.Answers))
		for _, answer := range job.ReportInput.Answers {
			if answer.Text != nil {
				answers[answer.QuestionNumber] = *answer.Text
			}
		}
		raw = structured.RestoreReportEvidenceCase(raw, answers)
		result, err := report.Parse(raw, job.ReportInput)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	}})
	result.Report = result.Evaluation
	result.Evaluation = nil
	return result, failure
}
