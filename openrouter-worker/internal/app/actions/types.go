package actions

import (
	"context"
	"encoding/json"
	feedback "hr-assistant/shared/candidatefeedback"
	report "hr-assistant/shared/candidatereport"
	"hr-assistant/shared/evaluation"
	"time"

	"github.com/google/uuid"
)

const VoiceTranscription = "voice_transcription"

type CompletedVoiceAnswer struct {
	AnswerID       uuid.UUID `json:"answer_id"`
	TelegramFileID string    `json:"telegram_file_id"`
	MimeType       string    `json:"mime_type,omitempty"`
	FileSize       int64     `json:"file_size,omitempty"`
	Duration       int       `json:"duration_seconds,omitempty"`
}

type InterviewCompletedPayload struct {
	SessionID      uuid.UUID              `json:"session_id"`
	VacancyID      uuid.UUID              `json:"vacancy_id"`
	TelegramChatID int64                  `json:"telegram_chat_id"`
	Language       string                 `json:"language,omitempty"`
	VoiceAnswers   []CompletedVoiceAnswer `json:"voice_answers"`
}

type Job struct {
	FeedbackInput    feedback.Input
	ReportInput      report.Input
	Version          int
	InvalidAttempts  int
	LastErrorMessage string
	EvaluationInput  evaluation.Input
	PromptVersion    string
	SchemaVersion    string
	ID               uuid.UUID
	ActionType       string
	AnswerID         uuid.UUID
	SessionID        uuid.UUID
	PartitionKey     int64
	Attempts         int
	TelegramFileID   string
	MimeType         string
	FileSize         int64
	Duration         int
	Language         string
	Model            string
}

type Usage struct {
	Seconds      *float64 `json:"seconds,omitempty"`
	TotalTokens  *int64   `json:"total_tokens,omitempty"`
	InputTokens  *int64   `json:"input_tokens,omitempty"`
	OutputTokens *int64   `json:"output_tokens,omitempty"`
	Cost         *float64 `json:"cost,omitempty"`
}

type Result struct {
	Feedback     json.RawMessage
	Report       json.RawMessage
	Evaluation   json.RawMessage
	Text         string
	Model        string
	GenerationID string
	Usage        Usage
}

type Failure struct {
	Permanent  bool
	Code       string
	Message    string
	RetryAfter time.Duration
}

type Handler interface {
	Type() string
	Execute(context.Context, Job) (Result, *Failure)
}

type Registry struct {
	handlers map[string]Handler
}

func NewRegistry(handlers ...Handler) Registry {
	registry := Registry{handlers: make(map[string]Handler, len(handlers))}
	for _, handler := range handlers {
		registry.handlers[handler.Type()] = handler
	}
	return registry
}

func (r Registry) Handler(actionType string) (Handler, bool) {
	handler, ok := r.handlers[actionType]
	return handler, ok
}
