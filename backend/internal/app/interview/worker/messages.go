package worker

import "github.com/google/uuid"

const (
	StartRequested                    = "candidate.start.requested.v1"
	SwitchRequested                   = "candidate.switch.requested.v1"
	ContinueRequested                 = "candidate.continue.requested.v1"
	CancelRequested                   = "candidate.cancel.requested.v1"
	AnswerReceived                    = "candidate.answer.received.v1"
	DeliverySucceeded                 = "candidate.delivery.succeeded.v1"
	DeliveryFailed                    = "candidate.delivery.failed.v1"
	TelegramQuestion                  = "telegram.question.send.v1"
	TelegramSystem                    = "telegram.system.send.v1"
	TelegramConflict                  = "telegram.session_conflict.send.v1"
	InterviewCompleted                = "interview.completed.v1"
	InterviewVoiceTranscribed         = "interview.voice.transcribed.v1"
	InterviewVoiceTranscriptionFailed = "interview.voice.transcription_failed.v1"
)

type CandidateIdentity struct {
	TelegramUserID int64   `json:"telegram_user_id"`
	TelegramChatID int64   `json:"telegram_chat_id"`
	Username       *string `json:"username,omitempty"`
	FirstName      string  `json:"first_name"`
	LastName       *string `json:"last_name,omitempty"`
	IsPremium      bool    `json:"is_premium"`
	LanguageCode   *string `json:"language_code,omitempty"`
}

type StartPayload struct {
	CandidateIdentity
	PublicToken string `json:"public_token"`
}

type AnswerPayload struct {
	CandidateIdentity
	TelegramMessageID int64  `json:"telegram_message_id"`
	AnswerType        string `json:"answer_type"`
	Text              string `json:"text,omitempty"`
	VoiceFileID       string `json:"voice_file_id,omitempty"`
	VoiceMimeType     string `json:"voice_mime_type,omitempty"`
	VoiceFileSize     int64  `json:"voice_file_size,omitempty"`
	VoiceDuration     int    `json:"voice_duration_seconds,omitempty"`
}

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

type TranscriptionUsage struct {
	Seconds      *float64 `json:"seconds,omitempty"`
	TotalTokens  *int64   `json:"total_tokens,omitempty"`
	InputTokens  *int64   `json:"input_tokens,omitempty"`
	OutputTokens *int64   `json:"output_tokens,omitempty"`
	Cost         *float64 `json:"cost,omitempty"`
}

type VoiceTranscribedPayload struct {
	AnswerID     uuid.UUID          `json:"answer_id"`
	SessionID    uuid.UUID          `json:"session_id"`
	Text         string             `json:"text"`
	Model        string             `json:"model"`
	Attempts     int                `json:"attempts"`
	GenerationID string             `json:"generation_id,omitempty"`
	Usage        TranscriptionUsage `json:"usage"`
}

type VoiceTranscriptionFailedPayload struct {
	AnswerID  uuid.UUID `json:"answer_id"`
	SessionID uuid.UUID `json:"session_id"`
	Model     string    `json:"model"`
	Attempts  int       `json:"attempts"`
	Code      string    `json:"code"`
	Message   string    `json:"message"`
}

type DeliverySucceededPayload struct {
	DeliveryID        uuid.UUID `json:"delivery_id"`
	TelegramMessageID int64     `json:"telegram_message_id"`
}

type DeliveryFailedPayload struct {
	DeliveryID uuid.UUID `json:"delivery_id"`
	Code       string    `json:"code"`
	Message    string    `json:"message"`
}

type InlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type TelegramSendPayload struct {
	DeliveryID     uuid.UUID        `json:"delivery_id"`
	SessionID      *uuid.UUID       `json:"session_id,omitempty"`
	ChatID         int64            `json:"chat_id"`
	Text           string           `json:"text"`
	AnswerType     string           `json:"answer_type,omitempty"`
	QuestionNumber int              `json:"question_number,omitempty"`
	QuestionTotal  int              `json:"question_total,omitempty"`
	Buttons        [][]InlineButton `json:"buttons,omitempty"`
}
