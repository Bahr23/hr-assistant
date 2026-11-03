package actions

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"hr-assistant/openrouter-worker/internal/core/database"
	"hr-assistant/openrouter-worker/internal/core/messaging"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRepositoryDeduplicatesJobsRecoversLeaseAndKeepsCompletedAnswersDone(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	var relation *string
	require.NoError(t, pool.QueryRow(ctx, "SELECT to_regclass('openrouter_worker.action_jobs')::text").Scan(&relation))
	if relation == nil {
		t.Skip("openrouter-worker migrations are not applied")
	}

	repository := NewRepository(pool)
	sessionID := uuid.New()
	messageID := uuid.New()
	answerIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
	payload := InterviewCompletedPayload{
		SessionID: sessionID, VacancyID: uuid.New(), TelegramChatID: time.Now().UnixNano(), Language: "ru",
		VoiceAnswers: []CompletedVoiceAnswer{
			{AnswerID: answerIDs[0], TelegramFileID: "voice-1", MimeType: "audio/ogg", FileSize: 101},
			{AnswerID: answerIDs[1], TelegramFileID: "voice-2", MimeType: "audio/ogg", FileSize: 102},
			{AnswerID: answerIDs[2], TelegramFileID: "voice-3", MimeType: "audio/ogg", FileSize: 103},
		},
	}
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	envelope := messaging.Envelope{
		MessageID: messageID, Type: messaging.InterviewCompleted, SchemaVersion: 1,
		OccurredAt: time.Now().UTC(), CorrelationID: sessionID,
		PartitionKey: payload.TelegramChatID, Payload: encoded,
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM outbox_events WHERE body->>'correlation_id' = $1", sessionID.String())
		_, _ = pool.Exec(ctx, "DELETE FROM action_jobs WHERE session_id = $1", sessionID)
		_, _ = pool.Exec(ctx, "DELETE FROM inbox_events WHERE message_id = $1", messageID)
	})

	created, err := repository.IngestCompletion(ctx, envelope, "test-model")
	require.NoError(t, err)
	require.Equal(t, 3, created)
	created, err = repository.IngestCompletion(ctx, envelope, "test-model")
	require.NoError(t, err)
	require.Zero(t, created)

	first, found, err := repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 1, first.Attempts)
	// Simulate a process crash after claim. The expired lease makes the same job recoverable.
	_, err = pool.Exec(ctx, `
UPDATE action_jobs SET lease_expires_at = now() - interval '1 second' WHERE id = $1`, first.ID)
	require.NoError(t, err)
	recovered, found, err := repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, first.ID, recovered.ID)
	require.Equal(t, 2, recovered.Attempts)
	require.NoError(t, repository.Succeed(ctx, recovered, Result{Text: "first", Model: "test-model"}))

	second, found, err := repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, repository.Retry(ctx, second,
		Failure{Code: "temporary", Message: "temporary failure"}, time.Now().Add(-time.Second)))
	secondRetry, found, err := repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, second.ID, secondRetry.ID)
	require.Equal(t, 2, secondRetry.Attempts)
	require.NoError(t, repository.Succeed(ctx, secondRetry, Result{Text: "second", Model: "test-model"}))

	third, found, err := repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, repository.PermanentFail(ctx, third,
		Failure{Permanent: true, Code: "unsupported_audio", Message: "unsupported audio"}))

	_, found, err = repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.False(t, found)
	var succeeded, failed, outbox int
	require.NoError(t, pool.QueryRow(ctx, `
SELECT count(*) FILTER (WHERE status = 'succeeded'),
       count(*) FILTER (WHERE status = 'permanent_failed')
FROM action_jobs WHERE session_id = $1`, sessionID).Scan(&succeeded, &failed))
	require.Equal(t, 2, succeeded)
	require.Equal(t, 1, failed)
	require.NoError(t, pool.QueryRow(ctx, `
SELECT count(*) FROM outbox_events WHERE body->>'correlation_id' = $1`, sessionID.String()).Scan(&outbox))
	require.Equal(t, 3, outbox)
}
