package service

import (
	"context"
	"strings"
	"time"

	"hr-assistant/backend/internal/app/candidatereport"
	"hr-assistant/backend/internal/core/domain"
	"hr-assistant/shared/evaluation"

	"github.com/google/uuid"
)

type store interface {
	List(context.Context, uuid.UUID, ListQuery) (Page, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (Interview, error)
	RetryEvaluation(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int) (int, error)
	RebuildReport(context.Context, uuid.UUID, uuid.UUID, int) (int, error)
}

type Service struct {
	store store
}

func New(store store) *Service {
	return &Service{store: store}
}

type ListQuery struct {
	Search    string
	VacancyID *uuid.UUID
	Limit     int32
	Offset    int32
}

type Profile struct {
	TelegramUserID int64
	Username       *string
	FirstName      string
	LastName       *string
	IsPremium      bool
	LanguageCode   *string
	FirstSeenAt    time.Time
	LastSeenAt     time.Time
}

type Vacancy struct {
	ID    uuid.UUID
	Title string
}

type Summary struct {
	ID              uuid.UUID
	Candidate       Profile
	Vacancy         Vacancy
	StartedAt       time.Time
	CompletedAt     time.Time
	DurationSeconds int64
	AnswerCount     int
}

type Page struct {
	Items  []Summary
	Total  int64
	Limit  int32
	Offset int32
}

type Transcription struct {
	Status string
	Text   *string
}

type Evaluation struct {
	Attempts      int
	ErrorCode     *string
	Status        string
	Result        *evaluation.Result
	Version       *int
	ResultVersion *int
	Reason        *string
}

type Answer struct {
	Evaluation    Evaluation
	ID            uuid.UUID
	Position      int
	Question      string
	AnswerType    string
	AnsweredAt    time.Time
	Text          *string
	Transcription *Transcription
}

type Interview struct {
	Report          candidatereport.View
	ID              uuid.UUID
	Candidate       Profile
	Vacancy         Vacancy
	StartedAt       time.Time
	CompletedAt     time.Time
	DurationSeconds int64
	Answers         []Answer
}

func (s *Service) List(ctx context.Context, hrID uuid.UUID, query ListQuery) (Page, error) {
	query.Search = strings.TrimSpace(query.Search)
	if len([]rune(query.Search)) > 100 {
		return Page{}, domain.ValidationFault("validation_failed", "search", "must not exceed 100 characters")
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return Page{}, domain.ValidationFault("validation_failed", "pagination", "limit must be 1..100 and offset must be non-negative")
	}
	return s.store.List(ctx, hrID, query)
}

func (s *Service) Get(ctx context.Context, hrID, sessionID uuid.UUID) (Interview, error) {
	return s.store.Get(ctx, hrID, sessionID)
}

func (s *Service) RetryEvaluation(ctx context.Context, hrID, sessionID, answerID uuid.UUID, version int) (int, error) {
	if version < 1 || version >= 2147483647 {
		return 0, domain.ValidationFault("validation_failed", "version", "invalid evaluation version")
	}
	return s.store.RetryEvaluation(ctx, hrID, sessionID, answerID, version)
}

func (s *Service) RebuildReport(ctx context.Context, hrID, sessionID uuid.UUID, version int) (int, error) {
	if version < 0 || version >= 2147483647 {
		return 0, domain.ValidationFault("validation_failed", "version", "invalid report version")
	}
	return s.store.RebuildReport(ctx, hrID, sessionID, version)
}
