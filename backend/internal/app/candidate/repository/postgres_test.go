package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"hr-assistant/backend/internal/app/candidate/service"
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

type fixture struct {
	ownerID, foreignOwnerID uuid.UUID
	vacancyID               uuid.UUID
	sessionID               uuid.UUID
	telegramUserID          int64
}

func createFixture(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	value := fixture{
		ownerID: uuid.New(), foreignOwnerID: uuid.New(), vacancyID: uuid.New(),
		sessionID: uuid.New(), telegramUserID: time.Now().UnixNano(),
	}
	foreignVacancyID := uuid.New()
	foreignSessionID := uuid.New()
	activeSessionID := uuid.New()
	textQuestionID, voiceQuestionID := uuid.New(), uuid.New()
	textAnswerID, voiceAnswerID := uuid.New(), uuid.New()
	startedAt := time.Now().UTC().Add(-5 * time.Minute).Truncate(time.Microsecond)
	completedAt := startedAt.Add(2 * time.Minute)

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM interview_sessions WHERE telegram_user_id = $1", value.telegramUserID)
		_, _ = pool.Exec(ctx, "DELETE FROM telegram_candidates WHERE telegram_user_id = $1", value.telegramUserID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = ANY($1::uuid[])", []uuid.UUID{value.ownerID, value.foreignOwnerID})
	})
	mustExec := func(query string, args ...any) {
		_, err := pool.Exec(ctx, query, args...)
		require.NoError(t, err)
	}

	mustExec(`INSERT INTO users (id, email, password_hash) VALUES
    ($1, $2, 'unused'), ($3, $4, 'unused')`,
		value.ownerID, "candidate-owner-"+uuid.NewString()+"@example.com",
		value.foreignOwnerID, "candidate-foreign-"+uuid.NewString()+"@example.com",
	)
	mustExec(`INSERT INTO vacancies (id, owner_user_id, title) VALUES
    ($1, $2, 'Go Developer'), ($3, $4, 'Foreign')`,
		value.vacancyID, value.ownerID, foreignVacancyID, value.foreignOwnerID,
	)
	mustExec(`
INSERT INTO telegram_candidates (
    telegram_user_id, telegram_chat_id, username, first_name, last_name,
    is_premium, language_code, first_seen_at, last_seen_at
) VALUES ($1, $1, 'ivan', 'Ivan', 'Petrov', true, 'ru', $2, $3)`,
		value.telegramUserID, startedAt, completedAt,
	)
	mustExec(`
INSERT INTO interview_sessions (
    id, vacancy_id, telegram_user_id, telegram_chat_id, status,
    current_question_position, created_at, updated_at, completed_at, expires_at
) VALUES
    ($1, $2, $3, $3, 'completed', 2, $4, $5, $5, $5),
    ($6, $7, $3, $3, 'completed', 0, $4, $5, $5, $5),
    ($8, $2, $3, $3, 'active', 0, $4, $4, NULL, $5 + interval '1 hour')`,
		value.sessionID, value.vacancyID, value.telegramUserID, startedAt, completedAt,
		foreignSessionID, foreignVacancyID, activeSessionID,
	)
	mustExec(`
INSERT INTO interview_session_questions (id, session_id, text, answer_type, position) VALUES
    ($1, $2, 'Text question', 'text', 0),
    ($3, $2, 'Voice question', 'voice', 1)`,
		textQuestionID, value.sessionID, voiceQuestionID,
	)
	mustExec(`
INSERT INTO interview_answers (
    id, session_id, question_position, telegram_message_id, answer_type,
    text_answer, telegram_voice_file_id, created_at
) VALUES
    ($1, $2, 0, 1001, 'text', 'Text answer', NULL, $3::timestamptz + interval '30 seconds'),
    ($4, $2, 1, 1002, 'voice', NULL, 'voice-id', $3::timestamptz + interval '60 seconds')`,
		textAnswerID, value.sessionID, startedAt, voiceAnswerID,
	)
	mustExec(`
INSERT INTO interview_answer_transcriptions (
    answer_id, status, transcription_text, model, attempts, created_at,
    updated_at, succeeded_at
) VALUES ($1, 'succeeded', 'Voice transcript', 'model', 1, $2, $3, $3)`,
		voiceAnswerID, startedAt, completedAt,
	)
	return value
}

func TestStoreListsOnlyOwnedCompletedInterviews(t *testing.T) {
	pool := integrationPool(t)
	data := createFixture(t, pool)
	store := NewStore(pool)

	page, err := store.List(context.Background(), data.ownerID, service.ListQuery{
		Search: "PETR", VacancyID: &data.vacancyID, Limit: 20,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	require.Equal(t, data.sessionID, page.Items[0].ID)
	require.Equal(t, "ivan", *page.Items[0].Candidate.Username)
	require.Equal(t, 2, page.Items[0].AnswerCount)
}

func TestStoreReturnsOrderedAnswersAndTranscription(t *testing.T) {
	pool := integrationPool(t)
	data := createFixture(t, pool)
	store := NewStore(pool)

	interview, err := store.Get(context.Background(), data.ownerID, data.sessionID)

	require.NoError(t, err)
	require.Len(t, interview.Answers, 2)
	require.Equal(t, "Text answer", *interview.Answers[0].Text)
	require.Nil(t, interview.Answers[0].Transcription)
	require.Equal(t, "succeeded", interview.Answers[1].Transcription.Status)
	require.Equal(t, "Voice transcript", *interview.Answers[1].Transcription.Text)

	_, err = store.Get(context.Background(), data.foreignOwnerID, data.sessionID)
	var fault *domain.Fault
	require.ErrorAs(t, err, &fault)
	require.Equal(t, domain.FaultNotFound, fault.Kind)
}
