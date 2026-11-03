package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Session struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	RefreshTokenHash string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type VacancyStatus string

const (
	VacancyDraft     VacancyStatus = "draft"
	VacancyPublished VacancyStatus = "published"
	VacancyArchived  VacancyStatus = "archived"
)

func (s VacancyStatus) Valid() bool {
	return s == VacancyPublished || s == VacancyArchived
}

type Vacancy struct {
	Description              *string
	EvaluationCriteria       []EvaluationCriterion
	ID                       uuid.UUID
	OwnerUserID              uuid.UUID
	Title                    string
	InterviewLanguage        string
	WelcomeMessage           string
	InterviewDurationMinutes int
	ExpiresAt                *time.Time
	Status                   VacancyStatus
	PublicToken              *string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type QuestionAnswerType string

const (
	QuestionAnswerText  QuestionAnswerType = "text"
	QuestionAnswerVoice QuestionAnswerType = "voice"
)

func (t QuestionAnswerType) Valid() bool {
	return t == QuestionAnswerText || t == QuestionAnswerVoice
}

type Question struct {
	EvaluationGuide *string
	ID              uuid.UUID
	VacancyID       uuid.UUID
	Text            string
	AnswerType      QuestionAnswerType
	Position        int32
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
