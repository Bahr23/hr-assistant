package repository

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"hr-assistant/backend/internal/app/vacancy/service"
	"hr-assistant/backend/internal/core/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(context.Background()))
	t.Cleanup(pool.Close)
	return pool
}

func createTestHR(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	_, err := pool.Exec(context.Background(), `
INSERT INTO users (id, email, password_hash)
VALUES ($1, $2, 'unused')`, userID, "concurrent-"+uuid.NewString()+"@example.com")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})
	return userID
}

func TestDefinitionStoreSerializesConcurrentQuestionRemoval(t *testing.T) {
	pool := integrationPool(t)
	ownerID := createTestHR(t, pool)
	module := service.New(NewDefinitionStore(pool), "http://frontend.test")
	expiresAt := time.Now().UTC().Add(time.Hour)
	created, err := module.Change(context.Background(), ownerID, service.CreateActive{Fields: service.VacancyFields{
		Title: "Developer", InterviewLanguage: "ru",
		WelcomeMessage: "Welcome", ExpiresAt: &expiresAt,
	}, Questions: []service.QuestionFields{
		{Text: "First", AnswerType: domain.QuestionAnswerText},
		{Text: "Second", AnswerType: domain.QuestionAnswerVoice},
	}})
	require.NoError(t, err)
	vacancyID := created.Definition.Vacancy.ID
	first, second := created.Definition.Questions[0], created.Definition.Questions[1]

	start := make(chan struct{})
	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for _, questionID := range []uuid.UUID{first.ID, second.ID} {
		wait.Add(1)
		go func(id uuid.UUID) {
			defer wait.Done()
			<-start
			_, changeErr := module.Change(context.Background(), ownerID, service.RemoveQuestion{
				VacancyID: vacancyID, QuestionID: id,
			})
			errors <- changeErr
		}(questionID)
	}
	close(start)
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

func TestPublicEntryReportsExistingInterviewAttempt(t *testing.T) {
	pool := integrationPool(t)
	ownerID := createTestHR(t, pool)
	module := service.New(NewDefinitionStore(pool), "https://t.me/bot")
	created, err := module.Change(context.Background(), ownerID, service.CreateActive{
		Fields: service.VacancyFields{
			Title: "Developer", InterviewLanguage: "ru", WelcomeMessage: "Welcome",
		},
		Questions: []service.QuestionFields{{
			Text: "Experience?", AnswerType: domain.QuestionAnswerText,
		}},
	})
	require.NoError(t, err)
	vacancyID := created.Definition.Vacancy.ID
	var publicToken string
	require.NoError(t, pool.QueryRow(context.Background(),
		"SELECT public_token FROM vacancies WHERE id = $1", vacancyID,
	).Scan(&publicToken))
	telegramUserID := time.Now().UnixNano()

	entry, err := module.GetPublicEntry(context.Background(), publicToken, telegramUserID)
	require.NoError(t, err)
	require.False(t, entry.AlreadyAttempted)

	_, err = pool.Exec(context.Background(), `
INSERT INTO interview_attempt_claims (
    vacancy_id, telegram_user_id, first_session_id, created_at
) VALUES ($1, $2, $3, now())`, vacancyID, telegramUserID, uuid.New())
	require.NoError(t, err)

	entry, err = module.GetPublicEntry(context.Background(), publicToken, telegramUserID)
	require.NoError(t, err)
	require.Equal(t, "Welcome", entry.WelcomeMessage)
	require.True(t, entry.AlreadyAttempted)
}
