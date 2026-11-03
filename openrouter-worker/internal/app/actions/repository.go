package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"hr-assistant/openrouter-worker/internal/core/messaging"
	feedback "hr-assistant/shared/candidatefeedback"
	report "hr-assistant/shared/candidatereport"
	"hr-assistant/shared/evaluation"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInvalidEvent = errors.New("invalid event")

type Repository struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, now: func() time.Time { return time.Now().UTC() }}
}

func (r *Repository) IngestCompletion(
	ctx context.Context,
	envelope messaging.Envelope,
	model string,
) (int, error) {
	if envelope.MessageID == uuid.Nil || envelope.CorrelationID == uuid.Nil ||
		envelope.Type != messaging.InterviewCompleted || envelope.SchemaVersion != 1 {
		return 0, fmt.Errorf("%w: invalid envelope", ErrInvalidEvent)
	}
	var payload InterviewCompletedPayload
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return 0, fmt.Errorf("%w: decode payload: %v", ErrInvalidEvent, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return 0, fmt.Errorf("%w: trailing payload data", ErrInvalidEvent)
	}
	if payload.SessionID == uuid.Nil || payload.VacancyID == uuid.Nil || payload.TelegramChatID <= 0 ||
		envelope.PartitionKey != payload.TelegramChatID || strings.TrimSpace(model) == "" {
		return 0, fmt.Errorf("%w: missing interview identifiers", ErrInvalidEvent)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `
INSERT INTO inbox_events (message_id, message_type, received_at)
VALUES ($1, $2, $3)
ON CONFLICT (message_id) DO NOTHING`, envelope.MessageID, envelope.Type, r.now())
	if err != nil {
		return 0, err
	}
	if command.RowsAffected() == 0 {
		return 0, tx.Commit(ctx)
	}

	created := 0
	for _, answer := range payload.VoiceAnswers {
		if answer.AnswerID == uuid.Nil || strings.TrimSpace(answer.TelegramFileID) == "" ||
			answer.FileSize < 0 || answer.Duration < 0 {
			return 0, fmt.Errorf("%w: invalid voice answer", ErrInvalidEvent)
		}
		actionID := uuid.New()
		command, err := tx.Exec(ctx, `
INSERT INTO action_jobs (
    id, source_message_id, action_type, subject_id, session_id, partition_key,
    status, attempts, next_attempt_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, 'pending', 0, $7, $7, $7)
ON CONFLICT (action_type, subject_id, analysis_version) DO NOTHING`,
			actionID, envelope.MessageID, VoiceTranscription, answer.AnswerID,
			payload.SessionID, payload.TelegramChatID, r.now())
		if err != nil {
			return 0, err
		}
		if command.RowsAffected() == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO voice_transcription_jobs (
    action_id, telegram_file_id, mime_type, file_size, duration_seconds, language, model
) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, 0), NULLIF($5, 0), NULLIF($6, ''), $7)`,
			actionID, answer.TelegramFileID, answer.MimeType, answer.FileSize,
			answer.Duration, payload.Language, model); err != nil {
			return 0, err
		}
		created++
	}
	return created, tx.Commit(ctx)
}

func (r *Repository) Claim(ctx context.Context, lease time.Duration) (Job, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback(ctx)

	var job Job
	var evaluationInput, reportInput, feedbackInput []byte
	err = tx.QueryRow(ctx, `
SELECT jobs.id, jobs.action_type, jobs.subject_id, jobs.session_id,
       jobs.partition_key, jobs.attempts + 1,
       COALESCE(voice.telegram_file_id, ''), COALESCE(voice.mime_type, ''),
       COALESCE(voice.file_size, 0), COALESCE(voice.duration_seconds, 0),
       COALESCE(voice.language, ''), COALESCE(voice.model, evaluation.model, report.model, feedback.model),
       jobs.analysis_version, jobs.invalid_attempts, evaluation.input,
       COALESCE(evaluation.prompt_version, report.prompt_version, feedback.prompt_version, ''), COALESCE(evaluation.schema_version, report.schema_version, feedback.schema_version, ''), report.input, feedback.input,
       COALESCE(jobs.last_error_message, '')
FROM action_jobs AS jobs
LEFT JOIN voice_transcription_jobs AS voice ON voice.action_id = jobs.id
LEFT JOIN answer_evaluation_jobs AS evaluation ON evaluation.action_id = jobs.id
LEFT JOIN candidate_report_jobs AS report ON report.action_id = jobs.id
LEFT JOIN candidate_feedback_jobs AS feedback ON feedback.action_id = jobs.id
WHERE (
    jobs.status IN ('pending', 'retry_wait') AND jobs.next_attempt_at <= now()
) OR (
    jobs.status = 'processing' AND jobs.lease_expires_at <= now()
)
ORDER BY jobs.next_attempt_at, jobs.created_at
FOR UPDATE OF jobs SKIP LOCKED
LIMIT 1`).Scan(
		&job.ID, &job.ActionType, &job.AnswerID, &job.SessionID,
		&job.PartitionKey, &job.Attempts, &job.TelegramFileID, &job.MimeType,
		&job.FileSize, &job.Duration, &job.Language, &job.Model,
		&job.Version, &job.InvalidAttempts, &evaluationInput, &job.PromptVersion, &job.SchemaVersion, &reportInput, &feedbackInput,
		&job.LastErrorMessage,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	if feedbackInput != nil {
		if err := json.Unmarshal(feedbackInput, &job.FeedbackInput); err != nil {
			return Job{}, false, err
		}
	}
	if reportInput != nil {
		if err := json.Unmarshal(reportInput, &job.ReportInput); err != nil {
			return Job{}, false, err
		}
	}
	if evaluationInput != nil {
		if err := json.Unmarshal(evaluationInput, &job.EvaluationInput); err != nil {
			return Job{}, false, err
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE action_jobs
SET status = 'processing', attempts = $2,
    lease_expires_at = now() + make_interval(secs => $3), updated_at = now()
WHERE id = $1`, job.ID, job.Attempts, lease.Seconds()); err != nil {
		return Job{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

func (r *Repository) Retry(ctx context.Context, job Job, failure Failure, next time.Time) error {
	command, err := r.pool.Exec(ctx, `
UPDATE action_jobs
SET status = 'retry_wait', next_attempt_at = $2, lease_expires_at = NULL,
    last_error_code = $3, last_error_message = $4, updated_at = now(),
    invalid_attempts = invalid_attempts + CASE WHEN $3 = 'invalid_result' THEN 1 ELSE 0 END
WHERE id = $1 AND status = 'processing' AND attempts = $5`,
		job.ID, next, failure.Code, failure.Message, job.Attempts)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("job %s lease is no longer owned by attempt %d", job.ID, job.Attempts)
	}
	return nil
}

func (r *Repository) Succeed(ctx context.Context, job Job, result Result) error {
	if job.ActionType == feedback.Action {
		if _, err := feedback.Parse(result.Feedback); err != nil {
			return err
		}
		payload := feedback.Completion{SessionID: job.SessionID, Version: job.Version, PromptVersion: job.PromptVersion, SchemaVersion: job.SchemaVersion, Model: result.Model, Attempts: job.Attempts, Result: result.Feedback}
		return r.finish(ctx, job, "succeeded", feedback.Succeeded, payload, nil)
	}
	if job.ActionType == report.Action {
		if _, err := report.Parse(result.Report, job.ReportInput); err != nil {
			return err
		}
		usage, err := json.Marshal(result.Usage)
		if err != nil {
			return err
		}
		payload := report.Completion{SessionID: job.SessionID, Version: job.Version, PromptVersion: job.PromptVersion, SchemaVersion: job.SchemaVersion, Model: result.Model, Attempts: job.Attempts, Result: result.Report, Usage: usage}
		return r.finish(ctx, job, "succeeded", report.Succeeded, payload, nil)
	}
	if job.ActionType == evaluation.Action {
		if _, err := evaluation.Parse(result.Evaluation, job.EvaluationInput.Answer); err != nil {
			return err
		}
		payload := evaluation.Completion{AnswerID: job.AnswerID, SessionID: job.SessionID,
			Version: job.Version, PromptVersion: job.PromptVersion, SchemaVersion: job.SchemaVersion,
			Model: result.Model, Attempts: job.Attempts, Result: result.Evaluation}
		return r.finish(ctx, job, "succeeded", evaluation.Succeeded, payload, nil)
	}
	payload := map[string]any{
		"answer_id": job.AnswerID, "session_id": job.SessionID,
		"text": strings.TrimSpace(result.Text), "model": result.Model,
		"attempts": job.Attempts, "generation_id": result.GenerationID,
		"usage": result.Usage,
	}
	return r.finish(ctx, job, "succeeded", messaging.VoiceTranscribed, payload, nil)
}

func (r *Repository) PermanentFail(ctx context.Context, job Job, failure Failure) error {
	if job.ActionType == feedback.Action {
		payload := feedback.Completion{SessionID: job.SessionID, Version: job.Version, PromptVersion: job.PromptVersion, SchemaVersion: job.SchemaVersion, Model: job.Model, Attempts: job.Attempts, Code: failure.Code}
		return r.finish(ctx, job, "permanent_failed", feedback.Failed, payload, &failure)
	}
	if job.ActionType == report.Action {
		payload := report.Completion{SessionID: job.SessionID, Version: job.Version, PromptVersion: job.PromptVersion, SchemaVersion: job.SchemaVersion, Model: job.Model, Attempts: job.Attempts, Code: failure.Code}
		return r.finish(ctx, job, "permanent_failed", report.Failed, payload, &failure)
	}
	if job.ActionType == evaluation.Action {
		payload := evaluation.Completion{AnswerID: job.AnswerID, SessionID: job.SessionID,
			Version: job.Version, PromptVersion: job.PromptVersion, SchemaVersion: job.SchemaVersion,
			Model: job.Model, Attempts: job.Attempts, Code: failure.Code}
		return r.finish(ctx, job, "permanent_failed", evaluation.Failed, payload, &failure)
	}
	payload := map[string]any{
		"answer_id": job.AnswerID, "session_id": job.SessionID,
		"model": job.Model, "attempts": job.Attempts,
		"code": failure.Code, "message": failure.Message,
	}
	return r.finish(ctx, job, "permanent_failed", messaging.VoiceTranscriptionFailed, payload, &failure)
}

func (r *Repository) finish(
	ctx context.Context,
	job Job,
	status string,
	eventType string,
	payload any,
	failure *Failure,
) error {
	envelope, err := messaging.NewEnvelope(eventType, job.SessionID, job.PartitionKey, payload)
	if err != nil {
		return err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var code, message *string
	if failure != nil {
		code, message = &failure.Code, &failure.Message
	}
	command, err := tx.Exec(ctx, `
UPDATE action_jobs
SET status = $2, lease_expires_at = NULL, last_error_code = $3,
    last_error_message = $4, completed_at = now(), updated_at = now()
WHERE id = $1 AND status = 'processing' AND attempts = $5`,
		job.ID, status, code, message, job.Attempts)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return fmt.Errorf("job %s is no longer leased", job.ID)
	}
	if job.ActionType == feedback.Action {
		if _, err := tx.Exec(ctx, `INSERT INTO completed_candidate_feedback(session_id,version) VALUES($1,$2) ON CONFLICT DO NOTHING`, job.SessionID, job.Version); err != nil {
			return err
		}
	}
	if job.ActionType == report.Action {
		if _, err := tx.Exec(ctx, `INSERT INTO completed_candidate_reports(session_id,version) VALUES($1,$2) ON CONFLICT DO NOTHING`, job.SessionID, job.Version); err != nil {
			return err
		}
	}
	if job.ActionType == evaluation.Action {
		if _, err := tx.Exec(ctx, `INSERT INTO completed_evaluations (answer_id, version) VALUES ($1, $2) ON CONFLICT DO NOTHING`, job.AnswerID, job.Version); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO outbox_events (id, exchange_name, routing_key, body, created_at)
VALUES ($1, $2, $3, $4, now())`,
		envelope.MessageID, messaging.InterviewExchange, eventType, body); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type OutboxEvent struct {
	ID         uuid.UUID
	Exchange   string
	RoutingKey string
	Body       []byte
}

func (r *Repository) NextOutbox(ctx context.Context) (OutboxEvent, bool, error) {
	var event OutboxEvent
	err := r.pool.QueryRow(ctx, `
SELECT id, exchange_name, routing_key, body
FROM outbox_events
WHERE published_at IS NULL
ORDER BY created_at
LIMIT 1`).Scan(&event.ID, &event.Exchange, &event.RoutingKey, &event.Body)
	if errors.Is(err, pgx.ErrNoRows) {
		return OutboxEvent{}, false, nil
	}
	return event, err == nil, err
}

func (r *Repository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
UPDATE outbox_events SET published_at = now()
WHERE id = $1 AND published_at IS NULL`, id)
	return err
}

func (r *Repository) Cleanup(
	ctx context.Context,
	inboxRetention, outboxRetention, jobRetention time.Duration,
) (int64, error) {
	var deleted int64
	queries := []struct {
		query     string
		retention time.Duration
	}{
		{`WITH doomed AS (
    SELECT message_id FROM inbox_events
    WHERE received_at < now() - make_interval(secs => $1)
    ORDER BY received_at LIMIT 1000
)
DELETE FROM inbox_events WHERE message_id IN (SELECT message_id FROM doomed)`, inboxRetention},
		{`WITH doomed AS (
    SELECT id FROM outbox_events
    WHERE published_at IS NOT NULL
      AND published_at < now() - make_interval(secs => $1)
    ORDER BY published_at LIMIT 1000
)
DELETE FROM outbox_events WHERE id IN (SELECT id FROM doomed)`, outboxRetention},
		{`WITH doomed AS (
    SELECT id FROM action_jobs
    WHERE status IN ('succeeded', 'permanent_failed')
      AND completed_at < now() - make_interval(secs => $1)
    ORDER BY completed_at LIMIT 1000
)
DELETE FROM action_jobs WHERE id IN (SELECT id FROM doomed)`, jobRetention},
	}
	for _, item := range queries {
		command, err := r.pool.Exec(ctx, item.query, item.retention.Seconds())
		if err != nil {
			return deleted, err
		}
		deleted += command.RowsAffected()
	}
	return deleted, nil
}
