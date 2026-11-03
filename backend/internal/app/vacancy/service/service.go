package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"

	"hr-assistant/backend/internal/core/domain"

	"github.com/google/uuid"
)

type store interface {
	Create(context.Context, domain.InterviewDefinition) (domain.InterviewDefinition, error)
	List(context.Context, uuid.UUID, ListQuery) ([]domain.Vacancy, error)
	Get(context.Context, uuid.UUID, uuid.UUID) (domain.InterviewDefinition, error)
	GetPublicEntryByToken(context.Context, string, int64) (PublicEntry, error)
	Change(context.Context, uuid.UUID, uuid.UUID, func(*domain.InterviewDefinition) error) (domain.InterviewDefinition, error)
}

type Service struct {
	store          store
	telegramBotURL string
	now            func() time.Time
	newToken       func() (string, error)
}

func New(store store, telegramBotURL string) *Service {
	return &Service{
		store: store, telegramBotURL: strings.TrimRight(telegramBotURL, "/"),
		now: func() time.Time { return time.Now().UTC() }, newToken: randomToken,
	}
}

type ListQuery struct {
	Status *domain.VacancyStatus
	Limit  int32
	Offset int32
}

type Definition struct {
	Vacancy   Vacancy
	Questions []domain.Question
}

type Vacancy struct {
	Description              *string
	EvaluationCriteria       []domain.EvaluationCriterion
	ID                       uuid.UUID
	Title                    string
	InterviewLanguage        string
	WelcomeMessage           string
	InterviewDurationMinutes int
	ExpiresAt                *time.Time
	Status                   domain.VacancyStatus
	PublicLink               *string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

type ChangeResult struct {
	Definition *Definition
	Question   *domain.Question
}

type PublicEntry struct {
	Description      *string
	WelcomeMessage   string
	AlreadyAttempted bool
}

type Change interface{ interviewDefinitionChange() }

type CreateActive struct {
	Fields    VacancyFields
	Questions []QuestionFields
}
type PatchVacancy struct {
	VacancyID uuid.UUID
	Patch     domain.VacancyPatch
}
type Archive struct{ VacancyID uuid.UUID }
type Restore struct{ VacancyID uuid.UUID }
type RotatePublicLink struct{ VacancyID uuid.UUID }
type AddQuestion struct {
	EvaluationGuide *string
	VacancyID       uuid.UUID
	Text            string
	AnswerType      domain.QuestionAnswerType
}
type EditQuestion struct {
	EvaluationGuide    *string
	EvaluationGuideSet bool
	VacancyID          uuid.UUID
	QuestionID         uuid.UUID
	Text               string
	AnswerType         domain.QuestionAnswerType
}
type RemoveQuestion struct {
	VacancyID  uuid.UUID
	QuestionID uuid.UUID
}
type ReorderQuestions struct {
	VacancyID   uuid.UUID
	QuestionIDs []uuid.UUID
}

func (CreateActive) interviewDefinitionChange()     {}
func (PatchVacancy) interviewDefinitionChange()     {}
func (Archive) interviewDefinitionChange()          {}
func (Restore) interviewDefinitionChange()          {}
func (RotatePublicLink) interviewDefinitionChange() {}
func (AddQuestion) interviewDefinitionChange()      {}
func (EditQuestion) interviewDefinitionChange()     {}
func (RemoveQuestion) interviewDefinitionChange()   {}
func (ReorderQuestions) interviewDefinitionChange() {}

type VacancyFields struct {
	Description              *string
	EvaluationCriteria       []domain.EvaluationCriterion
	Title                    string
	InterviewLanguage        string
	WelcomeMessage           string
	InterviewDurationMinutes int
	ExpiresAt                *time.Time
}

type QuestionFields struct {
	EvaluationGuide *string
	Text            string
	AnswerType      domain.QuestionAnswerType
}

func (m *Service) List(ctx context.Context, hrID uuid.UUID, query ListQuery) ([]Vacancy, error) {
	if query.Status != nil && !query.Status.Valid() {
		return nil, domain.ValidationFault("validation_failed", "status", "is invalid")
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.Limit < 1 || query.Limit > 100 || query.Offset < 0 {
		return nil, domain.ValidationFault("validation_failed", "pagination", "limit must be 1..100 and offset must be non-negative")
	}
	values, err := m.store.List(ctx, hrID, query)
	if err != nil {
		return nil, err
	}
	result := make([]Vacancy, 0, len(values))
	for _, vacancy := range values {
		result = append(result, m.vacancyView(vacancy))
	}
	return result, nil
}

func (m *Service) Get(ctx context.Context, hrID, vacancyID uuid.UUID) (Definition, error) {
	value, err := m.store.Get(ctx, hrID, vacancyID)
	if err != nil {
		return Definition{}, err
	}
	return m.definitionView(value), nil
}

func (m *Service) GetPublicEntry(ctx context.Context, publicToken string, telegramUserID int64) (PublicEntry, error) {
	if len(publicToken) == 0 || len(publicToken) > 64 {
		return PublicEntry{}, domain.NewFault(domain.FaultNotFound, "vacancy_not_found", "vacancy not found")
	}
	for _, char := range publicToken {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return PublicEntry{}, domain.NewFault(domain.FaultNotFound, "vacancy_not_found", "vacancy not found")
		}
	}
	if telegramUserID <= 0 {
		return PublicEntry{}, domain.ValidationFault("validation_failed", "telegram_user_id", "must be positive")
	}
	return m.store.GetPublicEntryByToken(ctx, publicToken, telegramUserID)
}

func (m *Service) Change(ctx context.Context, hrID uuid.UUID, change Change) (ChangeResult, error) {
	switch value := change.(type) {
	case CreateActive:
		return m.createActive(ctx, hrID, value)
	case PatchVacancy:
		return m.mutate(ctx, hrID, value.VacancyID, func(definition *domain.InterviewDefinition) (*uuid.UUID, error) {
			return nil, definition.PatchVacancy(value.Patch, m.now())
		})
	case Archive:
		return m.mutate(ctx, hrID, value.VacancyID, func(definition *domain.InterviewDefinition) (*uuid.UUID, error) {
			return nil, definition.Archive(m.now())
		})
	case Restore:
		return m.mutateWithNewToken(ctx, hrID, value.VacancyID, func(definition *domain.InterviewDefinition, token string) error {
			return definition.Restore(token, m.now())
		})
	case RotatePublicLink:
		return m.mutateWithNewToken(ctx, hrID, value.VacancyID, func(definition *domain.InterviewDefinition, token string) error {
			return definition.RotatePublicLink(token, m.now())
		})
	case AddQuestion:
		questionID := uuid.New()
		return m.mutate(ctx, hrID, value.VacancyID, func(definition *domain.InterviewDefinition) (*uuid.UUID, error) {
			err := definition.AddQuestion(domain.Question{
				ID: questionID, Text: value.Text, AnswerType: value.AnswerType, EvaluationGuide: value.EvaluationGuide,
			}, m.now())
			return &questionID, err
		})
	case EditQuestion:
		return m.mutate(ctx, hrID, value.VacancyID, func(definition *domain.InterviewDefinition) (*uuid.UUID, error) {
			var guidePatch []*string
			if value.EvaluationGuideSet {
				guidePatch = append(guidePatch, value.EvaluationGuide)
			}
			_, err := definition.EditQuestion(value.QuestionID, value.Text, value.AnswerType, m.now(), guidePatch...)
			return &value.QuestionID, err
		})
	case RemoveQuestion:
		return m.mutate(ctx, hrID, value.VacancyID, func(definition *domain.InterviewDefinition) (*uuid.UUID, error) {
			return nil, definition.RemoveQuestion(value.QuestionID, m.now())
		})
	case ReorderQuestions:
		return m.mutate(ctx, hrID, value.VacancyID, func(definition *domain.InterviewDefinition) (*uuid.UUID, error) {
			return nil, definition.ReorderQuestions(value.QuestionIDs, m.now())
		})
	default:
		return ChangeResult{}, domain.NewFault(domain.FaultValidation, "unsupported_change", "unsupported interview definition change")
	}
}

func (m *Service) createActive(ctx context.Context, hrID uuid.UUID, command CreateActive) (ChangeResult, error) {
	now := m.now()
	if command.Fields.InterviewDurationMinutes == 0 {
		command.Fields.InterviewDurationMinutes = 15
	}
	definition := domain.InterviewDefinition{Vacancy: domain.Vacancy{
		ID: uuid.New(), OwnerUserID: hrID, Status: domain.VacancyDraft, CreatedAt: now, UpdatedAt: now,
	}}
	patch := domain.VacancyPatch{
		Description: command.Fields.Description, DescriptionSet: true, EvaluationCriteria: &command.Fields.EvaluationCriteria,
		Title: &command.Fields.Title, InterviewLanguage: &command.Fields.InterviewLanguage,
		WelcomeMessage:           &command.Fields.WelcomeMessage,
		InterviewDurationMinutes: &command.Fields.InterviewDurationMinutes,
		ExpiresAt:                command.Fields.ExpiresAt, ExpiresAtSet: true,
	}
	if err := definition.PatchVacancy(patch, now); err != nil {
		return ChangeResult{}, err
	}
	for _, question := range command.Questions {
		if err := definition.AddQuestion(domain.Question{
			ID: uuid.New(), Text: question.Text, AnswerType: question.AnswerType, EvaluationGuide: question.EvaluationGuide,
		}, now); err != nil {
			return ChangeResult{}, err
		}
	}
	token, err := m.newToken()
	if err != nil {
		return ChangeResult{}, err
	}
	if err := definition.Publish(token, now); err != nil {
		return ChangeResult{}, err
	}
	saved, err := m.store.Create(ctx, definition)
	if err != nil {
		return ChangeResult{}, err
	}
	view := m.definitionView(saved)
	return ChangeResult{Definition: &view}, nil
}

func (m *Service) mutate(
	ctx context.Context,
	hrID, vacancyID uuid.UUID,
	mutation func(*domain.InterviewDefinition) (*uuid.UUID, error),
) (ChangeResult, error) {
	var changedQuestionID *uuid.UUID
	definition, err := m.store.Change(ctx, hrID, vacancyID, func(definition *domain.InterviewDefinition) error {
		var mutationErr error
		changedQuestionID, mutationErr = mutation(definition)
		return mutationErr
	})
	if err != nil {
		return ChangeResult{}, err
	}
	view := m.definitionView(definition)
	result := ChangeResult{Definition: &view}
	if changedQuestionID != nil {
		for index := range view.Questions {
			if view.Questions[index].ID == *changedQuestionID {
				question := view.Questions[index]
				result.Question = &question
				break
			}
		}
	}
	return result, nil
}

func (m *Service) mutateWithNewToken(
	ctx context.Context,
	hrID, vacancyID uuid.UUID,
	mutation func(*domain.InterviewDefinition, string) error,
) (ChangeResult, error) {
	var lastErr error
	for range 3 {
		token, err := m.newToken()
		if err != nil {
			return ChangeResult{}, err
		}
		result, err := m.mutate(ctx, hrID, vacancyID, func(definition *domain.InterviewDefinition) (*uuid.UUID, error) {
			return nil, mutation(definition, token)
		})
		if err == nil {
			return result, nil
		}
		var value *domain.Fault
		if !errors.As(err, &value) || value.Code != "conflict" {
			return ChangeResult{}, err
		}
		lastErr = err
	}
	return ChangeResult{}, lastErr
}

func (m *Service) definitionView(value domain.InterviewDefinition) Definition {
	return Definition{Vacancy: m.vacancyView(value.Vacancy), Questions: value.Questions}
}

func (m *Service) vacancyView(value domain.Vacancy) Vacancy {
	var publicLink *string
	if value.PublicToken != nil {
		telegramURL, _ := url.Parse(m.telegramBotURL)
		query := telegramURL.Query()
		query.Set("start", *value.PublicToken)
		telegramURL.RawQuery = query.Encode()
		link := telegramURL.String()
		publicLink = &link
	}
	return Vacancy{
		Description: value.Description, EvaluationCriteria: value.EvaluationCriteria,
		ID: value.ID, Title: value.Title, InterviewLanguage: value.InterviewLanguage,
		WelcomeMessage: value.WelcomeMessage, InterviewDurationMinutes: value.InterviewDurationMinutes,
		ExpiresAt: value.ExpiresAt, Status: value.Status,
		PublicLink: publicLink, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func randomToken() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
