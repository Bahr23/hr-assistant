package actions

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"hr-assistant/openrouter-worker/internal/core/database"
	"hr-assistant/openrouter-worker/internal/core/messaging"
	"hr-assistant/shared/evaluation"
)

func TestEvaluationJobsAreVersionedAndIdempotentAfterCleanup(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()
	repository := NewRepository(pool)
	request := evaluation.Request{AnswerID: uuid.New(), SessionID: uuid.New(), Version: 1,
		PromptVersion: evaluation.PromptVersion, SchemaVersion: evaluation.SchemaVersion,
		Input: evaluation.Input{Question: "Вопрос", Guide: "Пример", Answer: "Я спросил клиента", AnswerSource: "text"}}
	var messages []uuid.UUID
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_events WHERE body->>'correlation_id'=$1`, request.SessionID.String())
		_, _ = pool.Exec(ctx, `DELETE FROM action_jobs WHERE session_id=$1`, request.SessionID)
		_, _ = pool.Exec(ctx, `DELETE FROM completed_evaluations WHERE answer_id=$1`, request.AnswerID)
		_, _ = pool.Exec(ctx, `DELETE FROM inbox_events WHERE message_id=ANY($1)`, messages)
	}()
	ingest := func() int {
		envelope, err := messaging.NewEnvelope(evaluation.Requested, request.SessionID, 100, request)
		require.NoError(t, err)
		messages = append(messages, envelope.MessageID)
		count, err := repository.IngestEvaluation(ctx, envelope, "test-model")
		require.NoError(t, err)
		duplicate, err := repository.IngestEvaluation(ctx, envelope, "test-model")
		require.NoError(t, err)
		require.Zero(t, duplicate)
		return count
	}
	require.Equal(t, 1, ingest())
	require.Zero(t, ingest())
	job, found, err := repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, evaluation.Action, job.ActionType)
	require.Equal(t, request.Input, job.EvaluationInput)
	require.Equal(t, 1, job.Version)
	require.NoError(t, repository.Retry(ctx, job, Failure{Code: "invalid_result", Message: "invalid_result:unexpected_evidence"}, time.Now().Add(-time.Second)))
	job, found, err = repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 1, job.InvalidAttempts)
	require.Equal(t, "invalid_result:unexpected_evidence", job.LastErrorMessage)
	stale := job
	_, err = pool.Exec(ctx, `UPDATE action_jobs SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, job.ID)
	require.NoError(t, err)
	job, found, err = repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	result := Result{Model: "test-model", Evaluation: json.RawMessage(`{"summary":"Уточнил причину.","checks":[],"follow_up_questions":[]}`)}
	require.Error(t, repository.Succeed(ctx, stale, result))
	require.NoError(t, repository.Succeed(ctx, job, result))
	var body []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT body->'payload' FROM outbox_events WHERE body->>'type'=$1 AND body->>'correlation_id'=$2`, evaluation.Succeeded, request.SessionID.String()).Scan(&body))
	var completed evaluation.Completion
	require.NoError(t, json.Unmarshal(body, &completed))
	require.Equal(t, request.AnswerID, completed.AnswerID)
	require.Equal(t, 1, completed.Version)
	require.Equal(t, 3, completed.Attempts)
	// Retention removes job details, but replay must not repeat a paid AI request.
	_, err = pool.Exec(ctx, `DELETE FROM action_jobs WHERE session_id=$1`, request.SessionID)
	require.NoError(t, err)
	require.Zero(t, ingest())
	request.Version = 2
	require.Equal(t, 1, ingest())
	require.Zero(t, ingest())
	job, found, err = repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 2, job.Version)
	require.NoError(t, repository.PermanentFail(ctx, job, Failure{Permanent: true, Code: "invalid_result", Message: "invalid_result"}))
	require.Zero(t, ingest())
}
