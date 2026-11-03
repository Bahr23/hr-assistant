package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hr-assistant/backend/internal/app/answerevaluation"
	"hr-assistant/backend/internal/app/candidatefeedback"
	"hr-assistant/backend/internal/app/candidatereport"
	"hr-assistant/backend/internal/core/messaging"
	feedback "hr-assistant/shared/candidatefeedback"
	report "hr-assistant/shared/candidatereport"
	"hr-assistant/shared/evaluation"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (p *Processor) ProcessAIResult(
	ctx context.Context,
	envelope messaging.Envelope,
) (permanent bool, err error) {
	if envelope.MessageID == uuid.Nil || envelope.CorrelationID == uuid.Nil || envelope.SchemaVersion != 1 {
		return true, fmt.Errorf("invalid AI result envelope")
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	command, err := tx.Exec(ctx, `
INSERT INTO inbox_messages (message_id, message_type)
VALUES ($1, $2)
ON CONFLICT (message_id) DO NOTHING`, envelope.MessageID, envelope.Type)
	if err != nil {
		return false, err
	}
	if command.RowsAffected() == 0 {
		return false, tx.Commit(ctx)
	}

	now := p.now()
	var answerID, sessionID uuid.UUID
	switch envelope.Type {
	case feedback.Succeeded, feedback.Failed:
		var payload feedback.Completion
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		text, permanent, err := candidatefeedback.SaveResult(ctx, tx, envelope.Type, envelope.PartitionKey, payload, now)
		if err != nil {
			return permanent, err
		}
		if text != "" {
			if err := p.enqueueSystem(ctx, tx, envelope, envelope.PartitionKey, &payload.SessionID, text); err != nil {
				return false, err
			}
		}
		return false, tx.Commit(ctx)
	case report.Succeeded, report.Failed:
		var payload report.Completion
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		permanent, err := candidatereport.SaveResult(ctx, tx, envelope.Type, envelope.PartitionKey, payload, now)
		if err != nil {
			return permanent, err
		}
		return false, tx.Commit(ctx)
	case evaluation.Succeeded, evaluation.Failed:
		var payload evaluation.Completion
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		if err := lockAIResultSession(ctx, tx, payload.SessionID, envelope.PartitionKey); err != nil {
			return errors.Is(err, pgx.ErrNoRows), err
		}
		permanent, err := answerevaluation.SaveResult(ctx, tx, envelope.Type, envelope.PartitionKey, payload, now)
		if err != nil {
			return permanent, err
		}
		if err := candidatereport.ScheduleReady(ctx, tx, payload.SessionID, now); err != nil {
			return false, err
		}
		return false, tx.Commit(ctx)
	case InterviewVoiceTranscribed:
		var payload VoiceTranscribedPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		if payload.AnswerID == uuid.Nil || payload.SessionID == uuid.Nil ||
			strings.TrimSpace(payload.Text) == "" || strings.TrimSpace(payload.Model) == "" || payload.Attempts < 1 {
			return true, fmt.Errorf("invalid voice transcription result")
		}
		answerID, sessionID = payload.AnswerID, payload.SessionID
		if err := lockAIResultSession(ctx, tx, sessionID, envelope.PartitionKey); err != nil {
			return errors.Is(err, pgx.ErrNoRows), err
		}
		command, err = tx.Exec(ctx, `
UPDATE interview_answer_transcriptions AS transcriptions
SET status = 'succeeded', transcription_text = $3, model = $4, attempts = $5,
    generation_id = NULLIF($6, ''), usage_seconds = $7, usage_total_tokens = $8,
    usage_input_tokens = $9, usage_output_tokens = $10, usage_cost_usd = $11,
    error_code = NULL, error_message = NULL, updated_at = $12,
    succeeded_at = $12, failed_at = NULL
FROM interview_answers AS answers
WHERE transcriptions.answer_id = $1
  AND answers.id = transcriptions.answer_id
  AND answers.session_id = $2
  AND answers.answer_type = 'voice'
  AND transcriptions.status <> 'succeeded'`,
			payload.AnswerID, payload.SessionID, strings.TrimSpace(payload.Text), payload.Model,
			payload.Attempts, payload.GenerationID, payload.Usage.Seconds, payload.Usage.TotalTokens,
			payload.Usage.InputTokens, payload.Usage.OutputTokens, payload.Usage.Cost, now)
	case InterviewVoiceTranscriptionFailed:
		var payload VoiceTranscriptionFailedPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		if payload.AnswerID == uuid.Nil || payload.SessionID == uuid.Nil ||
			strings.TrimSpace(payload.Code) == "" || strings.TrimSpace(payload.Message) == "" || payload.Attempts < 1 {
			return true, fmt.Errorf("invalid voice transcription failure")
		}
		answerID, sessionID = payload.AnswerID, payload.SessionID
		if err := lockAIResultSession(ctx, tx, sessionID, envelope.PartitionKey); err != nil {
			return errors.Is(err, pgx.ErrNoRows), err
		}
		command, err = tx.Exec(ctx, `
UPDATE interview_answer_transcriptions AS transcriptions
SET status = 'permanent_failed', model = NULLIF($3, ''), attempts = $4,
    transcription_text = NULL, generation_id = NULL,
    usage_seconds = NULL, usage_total_tokens = NULL, usage_input_tokens = NULL,
    usage_output_tokens = NULL, usage_cost_usd = NULL,
    error_code = $5, error_message = $6, updated_at = $7,
    succeeded_at = NULL, failed_at = $7
FROM interview_answers AS answers
WHERE transcriptions.answer_id = $1
  AND answers.id = transcriptions.answer_id
  AND answers.session_id = $2
  AND answers.answer_type = 'voice'
  AND transcriptions.status <> 'succeeded'`,
			payload.AnswerID, payload.SessionID, payload.Model, payload.Attempts,
			payload.Code, payload.Message, now)
	default:
		return true, fmt.Errorf("unsupported AI result type %q", envelope.Type)
	}
	if err != nil {
		return false, err
	}
	if command.RowsAffected() == 0 {
		var status string
		lookupErr := tx.QueryRow(ctx, `
SELECT transcriptions.status
FROM interview_answer_transcriptions AS transcriptions
JOIN interview_answers AS answers ON answers.id = transcriptions.answer_id
WHERE transcriptions.answer_id = $1 AND answers.session_id = $2`, answerID, sessionID).Scan(&status)
		if lookupErr == nil && status == "succeeded" {
			return false, tx.Commit(ctx)
		}
		if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
			return false, lookupErr
		}
		return true, fmt.Errorf("voice transcription target does not exist")
	}
	if envelope.Type == InterviewVoiceTranscribed {
		if err := answerevaluation.ScheduleReady(ctx, tx, sessionID, now); err != nil {
			return false, err
		}
	}
	if err := candidatereport.ScheduleReady(ctx, tx, sessionID, now); err != nil {
		return false, err
	}
	return false, tx.Commit(ctx)
}

// All answer results and retries lock the session before touching dependent rows.
// This also authenticates the voice event partition against its recorded session.
func lockAIResultSession(ctx context.Context, tx pgx.Tx, id uuid.UUID, partition int64) error {
	var locked uuid.UUID
	return tx.QueryRow(ctx, `SELECT id FROM interview_sessions WHERE id=$1 AND telegram_chat_id=$2 AND status='completed' FOR UPDATE`, id, partition).Scan(&locked)
}
