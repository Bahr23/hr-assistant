package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"hr-assistant/backend/internal/core/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type memoryDefinitionStore struct {
	mu          sync.Mutex
	definitions map[uuid.UUID]domain.InterviewDefinition
}

func newMemoryDefinitionStore() *memoryDefinitionStore {
	return &memoryDefinitionStore{definitions: map[uuid.UUID]domain.InterviewDefinition{}}
}

func (s *memoryDefinitionStore) Create(_ context.Context, value domain.InterviewDefinition) (domain.InterviewDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.definitions[value.Vacancy.ID] = cloneDefinition(value)
	return cloneDefinition(value), nil
}

func (s *memoryDefinitionStore) List(_ context.Context, ownerID uuid.UUID, _ ListQuery) ([]domain.Vacancy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := []domain.Vacancy{}
	for _, value := range s.definitions {
		if value.Vacancy.OwnerUserID == ownerID {
			result = append(result, value.Vacancy)
		}
	}
	return result, nil
}

func (s *memoryDefinitionStore) Get(_ context.Context, ownerID, vacancyID uuid.UUID) (domain.InterviewDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.definitions[vacancyID]
	if !ok || value.Vacancy.OwnerUserID != ownerID {
		return domain.InterviewDefinition{}, domain.NewFault(domain.FaultNotFound, "vacancy_not_found", "vacancy not found")
	}
	return cloneDefinition(value), nil
}

func (s *memoryDefinitionStore) GetPublicEntryByToken(_ context.Context, publicToken string, _ int64) (PublicEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, value := range s.definitions {
		if value.Vacancy.PublicToken != nil && *value.Vacancy.PublicToken == publicToken &&
			value.Vacancy.Status == domain.VacancyPublished &&
			(value.Vacancy.ExpiresAt == nil || value.Vacancy.ExpiresAt.After(time.Now())) {
			return PublicEntry{WelcomeMessage: value.Vacancy.WelcomeMessage}, nil
		}
	}
	return PublicEntry{}, domain.NewFault(domain.FaultNotFound, "vacancy_not_found", "vacancy not found")
}

func (s *memoryDefinitionStore) Change(_ context.Context, ownerID, vacancyID uuid.UUID, mutate func(*domain.InterviewDefinition) error) (domain.InterviewDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.definitions[vacancyID]
	if !ok || value.Vacancy.OwnerUserID != ownerID {
		return domain.InterviewDefinition{}, domain.NewFault(domain.FaultNotFound, "vacancy_not_found", "vacancy not found")
	}
	working := cloneDefinition(value)
	if err := mutate(&working); err != nil {
		return domain.InterviewDefinition{}, err
	}
	s.definitions[vacancyID] = cloneDefinition(working)
	return cloneDefinition(working), nil
}

func cloneDefinition(value domain.InterviewDefinition) domain.InterviewDefinition {
	result := value
	result.Questions = append([]domain.Question(nil), value.Questions...)
	return result
}

func TestConcurrentQuestionRemovalPreservesPublishedInvariant(t *testing.T) {
	store := newMemoryDefinitionStore()
	module := New(store, "http://frontend.test")
	ownerID := uuid.New()
	expiresAt := time.Now().UTC().Add(time.Hour)
	created, err := module.Change(context.Background(), ownerID, CreateActive{Fields: VacancyFields{
		Title: "Developer", InterviewLanguage: "ru",
		WelcomeMessage: "Welcome", ExpiresAt: &expiresAt,
	}, Questions: []QuestionFields{
		{Text: "First", AnswerType: domain.QuestionAnswerText},
		{Text: "Second", AnswerType: domain.QuestionAnswerVoice},
	}})
	require.NoError(t, err)
	vacancyID := created.Definition.Vacancy.ID
	first, second := created.Definition.Questions[0], created.Definition.Questions[1]

	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for _, id := range []uuid.UUID{first.ID, second.ID} {
		wait.Add(1)
		go func(questionID uuid.UUID) {
			defer wait.Done()
			_, changeErr := module.Change(context.Background(), ownerID, RemoveQuestion{VacancyID: vacancyID, QuestionID: questionID})
			errors <- changeErr
		}(id)
	}
	wait.Wait()
	close(errors)

	successes := 0
	for changeErr := range errors {
		if changeErr == nil {
			successes++
		}
	}
	require.Equal(t, 1, successes)
	definition, err := module.Get(context.Background(), ownerID, vacancyID)
	require.NoError(t, err)
	require.Equal(t, domain.VacancyPublished, definition.Vacancy.Status)
	require.Len(t, definition.Questions, 1)
}

func TestOwnershipIsIndistinguishableFromAbsence(t *testing.T) {
	store := newMemoryDefinitionStore()
	module := New(store, "http://frontend.test")
	ownerID := uuid.New()
	created, err := module.Change(context.Background(), ownerID, CreateActive{Fields: VacancyFields{
		Title: "Developer", InterviewLanguage: "ru", WelcomeMessage: "Welcome",
	}, Questions: []QuestionFields{{Text: "Experience?", AnswerType: domain.QuestionAnswerVoice}}})
	require.NoError(t, err)

	_, err = module.Get(context.Background(), uuid.New(), created.Definition.Vacancy.ID)

	var fault *domain.Fault
	require.ErrorAs(t, err, &fault)
	require.Equal(t, domain.FaultNotFound, fault.Kind)
}

func TestPublishedVacancyUsesTelegramDeepLink(t *testing.T) {
	store := newMemoryDefinitionStore()
	module := New(store, "https://t.me/bot")
	module.newToken = func() (string, error) { return "test", nil }
	ownerID := uuid.New()
	published, err := module.Change(context.Background(), ownerID, CreateActive{Fields: VacancyFields{
		Title: "Developer", InterviewLanguage: "ru", WelcomeMessage: "Welcome",
	}, Questions: []QuestionFields{{Text: "Experience?", AnswerType: domain.QuestionAnswerVoice}}})

	require.NoError(t, err)
	require.NotNil(t, published.Definition.Vacancy.PublicLink)
	require.Equal(t, "https://t.me/bot?start=test", *published.Definition.Vacancy.PublicLink)
}

func TestRandomTokenIsShortAndURLSafe(t *testing.T) {
	token, err := randomToken()
	require.NoError(t, err)
	require.Len(t, token, 22)
	for _, char := range token {
		require.True(t, (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_')
	}
}

func TestWelcomeMessageIsResolvedByPublicToken(t *testing.T) {
	store := newMemoryDefinitionStore()
	module := New(store, "https://t.me/bot")
	module.newToken = func() (string, error) { return "short-token", nil }
	created, err := module.Change(context.Background(), uuid.New(), CreateActive{Fields: VacancyFields{
		Title: "Developer", InterviewLanguage: "ru", WelcomeMessage: "Добро пожаловать!",
	}, Questions: []QuestionFields{{Text: "Experience?", AnswerType: domain.QuestionAnswerText}}})
	require.NoError(t, err)

	entry, err := module.GetPublicEntry(context.Background(), "short-token", 13)
	require.NoError(t, err)
	require.Equal(t, "Добро пожаловать!", entry.WelcomeMessage)
	require.False(t, entry.AlreadyAttempted)
	require.NotNil(t, created.Definition.Vacancy.PublicLink)
}
