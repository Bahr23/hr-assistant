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
	report "hr-assistant/shared/candidatereport"
)

func TestReportJobsAreVersionedAndIdempotentAfterCleanup(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()
	repository := NewRepository(pool)
	request := report.Request{SessionID: uuid.New(), Version: 1,
		PromptVersion: report.PromptVersion, SchemaVersion: report.SchemaVersion,
		Input: report.Input{VacancyTitle: "Продажи", Criteria: []report.Criterion{}, Answers: []report.Answer{{QuestionNumber: 1, Question: "Вопрос", Text: ptrReportText("Я спросил клиента"), Source: "text", EvaluationStatus: "skipped", TranscriptionStatus: "skipped"}}}}
	var messages []uuid.UUID
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_events WHERE body->>'correlation_id'=$1`, request.SessionID.String())
		_, _ = pool.Exec(ctx, `DELETE FROM action_jobs WHERE session_id=$1`, request.SessionID)
		_, _ = pool.Exec(ctx, `DELETE FROM completed_candidate_reports WHERE session_id=$1`, request.SessionID)
		_, _ = pool.Exec(ctx, `DELETE FROM inbox_events WHERE message_id=ANY($1)`, messages)
	}()
	ingest := func() int {
		envelope, err := messaging.NewEnvelope(report.Requested, request.SessionID, 100, request)
		require.NoError(t, err)
		messages = append(messages, envelope.MessageID)
		count, err := repository.IngestReport(ctx, envelope, "test-model")
		require.NoError(t, err)
		duplicate, err := repository.IngestReport(ctx, envelope, "test-model")
		require.NoError(t, err)
		require.Zero(t, duplicate)
		return count
	}
	require.Equal(t, 1, ingest())
	require.Zero(t, ingest())
	job, found, err := repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, report.Action, job.ActionType)
	require.Equal(t, request.Input, job.ReportInput)
	require.Equal(t, 1, job.Version)
	const diagnostic = "invalid_result:missing_evidence:criterion_scores[2].evidence"
	require.NoError(t, repository.Retry(ctx, job, Failure{Code: "invalid_result", Message: diagnostic}, time.Now().Add(-time.Second)))
	job, found, err = repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, 1, job.InvalidAttempts)
	require.Equal(t, diagnostic, job.LastErrorMessage)
	stale := job
	_, err = pool.Exec(ctx, `UPDATE action_jobs SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, job.ID)
	require.NoError(t, err)
	job, found, err = repository.Claim(ctx, time.Minute)
	require.NoError(t, err)
	require.True(t, found)
	result := Result{Model: "test-model", Report: json.RawMessage(`{"summary":"Уточнил причину.","criterion_scores":[],"strengths":[],"concerns":[],"missing_information":[],"follow_up_questions":[],"analysis_completeness":{"status":"complete","processed_answers":1,"total_answers":1,"issues":[]}}`)}
	require.Error(t, repository.Succeed(ctx, stale, result))
	require.NoError(t, repository.Succeed(ctx, job, result))
	var body []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT body->'payload' FROM outbox_events WHERE body->>'type'=$1 AND body->>'correlation_id'=$2`, report.Succeeded, request.SessionID.String()).Scan(&body))
	var completed report.Completion
	require.NoError(t, json.Unmarshal(body, &completed))
	require.Equal(t, request.SessionID, completed.SessionID)
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
	require.NoError(t, repository.PermanentFail(ctx, job, Failure{Permanent: true, Code: "invalid_result", Message: diagnostic}))
	var persisted string
	require.NoError(t, pool.QueryRow(ctx, `SELECT last_error_message FROM action_jobs WHERE id=$1`, job.ID).Scan(&persisted))
	require.Equal(t, diagnostic, persisted)
	require.Zero(t, ingest())
}

func ptrReportText(v string) *string { return &v }
