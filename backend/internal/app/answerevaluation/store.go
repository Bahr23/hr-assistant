// Package answerevaluation owns scheduling and persistence of answer checks.
// Every operation participates in its caller's transaction and outbox.
package answerevaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"hr-assistant/backend/internal/core/messaging"
	"hr-assistant/shared/evaluation"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const readyAnswers = `
SELECT answers.id, sessions.id, sessions.telegram_chat_id,
       questions.text, questions.evaluation_guide,
       CASE WHEN answers.answer_type = 'text' THEN answers.text_answer ELSE transcriptions.transcription_text END,
       CASE WHEN answers.answer_type = 'text' THEN 'text' ELSE 'transcription' END
FROM interview_answers AS answers
JOIN interview_sessions AS sessions ON sessions.id = answers.session_id
JOIN interview_session_questions AS questions ON questions.session_id = answers.session_id
    AND questions.position = answers.question_position
LEFT JOIN interview_answer_transcriptions AS transcriptions ON transcriptions.answer_id = answers.id
WHERE sessions.status = 'completed' AND questions.evaluation_guide IS NOT NULL
  AND ((answers.answer_type = 'text' AND answers.text_answer IS NOT NULL)
    OR (answers.answer_type = 'voice' AND transcriptions.status = 'succeeded'))`

func ScheduleReady(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, now time.Time) error {
	rows, err := tx.Query(ctx, readyAnswers+" AND sessions.id = $1 ORDER BY answers.question_position", sessionID)
	if err != nil {
		return err
	}
	type ready struct {
		request evaluation.Request
		chatID  int64
	}
	var answers []ready
	for rows.Next() {
		var answer ready
		r := &answer.request
		if err := rows.Scan(&r.AnswerID, &r.SessionID, &answer.chatID, &r.Input.Question, &r.Input.Guide, &r.Input.Answer, &r.Input.AnswerSource); err != nil {
			rows.Close()
			return err
		}
		r.Version, r.PromptVersion, r.SchemaVersion = 1, evaluation.PromptVersion, evaluation.SchemaVersion
		answers = append(answers, answer)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, answer := range answers {
		if err := schedule(ctx, tx, answer.request, answer.chatID, now); err != nil {
			return err
		}
	}
	return nil
}

func schedule(ctx context.Context, tx pgx.Tx, request evaluation.Request, chatID int64, now time.Time) error {
	if !request.Input.Valid() {
		return fmt.Errorf("invalid answer evaluation input")
	}
	command, err := tx.Exec(ctx, `
INSERT INTO interview_answer_evaluations (answer_id, version, status, prompt_version, schema_version, created_at, updated_at)
VALUES ($1, $2, 'pending', $3, $4, $5, $5)
ON CONFLICT (answer_id, version) DO NOTHING`, request.AnswerID, request.Version, request.PromptVersion, request.SchemaVersion, now)
	if err != nil || command.RowsAffected() == 0 {
		return err
	}
	envelope, err := messaging.NewEnvelope(evaluation.Requested, request.SessionID, chatID, request)
	if err != nil {
		return err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events (id, exchange_name, routing_key, body, created_at)
VALUES ($1, $2, $3, $4, $5)`, envelope.MessageID, messaging.InterviewExchange, evaluation.Requested, body, now)
	return err
}

var ErrNotRetryable = errors.New("evaluation is not retryable")

// Retry requires the observed failed version. Concurrent/repeated requests create
// exactly the same next version, even if that next version has already completed.
func Retry(ctx context.Context, tx pgx.Tx, hrID, sessionID, answerID uuid.UUID, version int, now time.Time) (int, error) {
	var locked uuid.UUID
	err := tx.QueryRow(ctx, `
SELECT answers.id FROM interview_answers AS answers
JOIN interview_sessions AS sessions ON sessions.id = answers.session_id
JOIN vacancies ON vacancies.id = sessions.vacancy_id
WHERE answers.id = $1 AND sessions.id = $2 AND sessions.status = 'completed' AND vacancies.owner_user_id = $3
FOR UPDATE OF sessions`, answerID, sessionID, hrID).Scan(&locked)
	if err != nil {
		return 0, err
	}
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM interview_answer_evaluations WHERE answer_id = $1 AND version = $2`, answerID, version).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotRetryable
	}
	if err != nil {
		return 0, err
	}
	if status != "permanent_failed" {
		return 0, ErrNotRetryable
	}
	var request evaluation.Request
	var chatID int64
	err = tx.QueryRow(ctx, readyAnswers+" AND answers.id = $1", answerID).Scan(
		&request.AnswerID, &request.SessionID, &chatID, &request.Input.Question, &request.Input.Guide, &request.Input.Answer, &request.Input.AnswerSource,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotRetryable
	}
	if err != nil {
		return 0, err
	}
	request.Version, request.PromptVersion, request.SchemaVersion = version+1, evaluation.PromptVersion, evaluation.SchemaVersion
	return request.Version, schedule(ctx, tx, request, chatID, now)
}

func SaveResult(ctx context.Context, tx pgx.Tx, eventType string, partitionKey int64, payload evaluation.Completion, now time.Time) (bool, error) {
	if payload.AnswerID == uuid.Nil || payload.SessionID == uuid.Nil || payload.Version < 1 || payload.Attempts < 1 ||
		strings.TrimSpace(payload.Model) == "" || len(payload.Model) > 255 {
		return true, fmt.Errorf("invalid evaluation completion")
	}
	var status, promptVersion, schemaVersion, source string
	err := tx.QueryRow(ctx, `
SELECT evaluations.status, evaluations.prompt_version, evaluations.schema_version,
       CASE WHEN answers.answer_type = 'text' THEN answers.text_answer ELSE transcriptions.transcription_text END
FROM interview_answer_evaluations AS evaluations
JOIN interview_answers AS answers ON answers.id = evaluations.answer_id
JOIN interview_sessions AS sessions ON sessions.id = answers.session_id
LEFT JOIN interview_answer_transcriptions AS transcriptions ON transcriptions.answer_id = answers.id
WHERE evaluations.answer_id = $1 AND evaluations.version = $2 AND sessions.id = $3
  AND sessions.telegram_chat_id = $4 AND sessions.status = 'completed'
FOR UPDATE OF evaluations`, payload.AnswerID, payload.Version, payload.SessionID, partitionKey).Scan(&status, &promptVersion, &schemaVersion, &source)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, fmt.Errorf("evaluation target does not exist")
	}
	if err != nil {
		return false, err
	}
	if payload.PromptVersion != promptVersion || payload.SchemaVersion != schemaVersion {
		return true, fmt.Errorf("evaluation version mismatch")
	}
	if status != "pending" {
		return false, nil
	}
	var result []byte
	var lastError *string
	switch eventType {
	case evaluation.Succeeded:
		validated, err := evaluation.Parse(payload.Result, source)
		if err != nil {
			return true, err
		}
		result, err = json.Marshal(validated)
		if err != nil {
			return true, err
		}
		status = "succeeded"
	case evaluation.Failed:
		if payload.Code == "" || len(payload.Result) != 0 {
			return true, fmt.Errorf("invalid evaluation failure")
		}
		status = "permanent_failed"
		code := "evaluation_failed"
		switch payload.Code {
		case "invalid_result", "unsupported_version", "invalid_input", "openrouter_rejected", "openrouter_auth_failed", "openrouter_unavailable", "model_refusal":
			code = payload.Code
		}
		lastError = &code
	default:
		return true, fmt.Errorf("unsupported evaluation completion")
	}
	_, err = tx.Exec(ctx, `
UPDATE interview_answer_evaluations SET status = $3, result = $4, model = $5,
    attempt_count = $6, last_error = $7, completed_at = $8, updated_at = $8
WHERE answer_id = $1 AND version = $2`, payload.AnswerID, payload.Version, status, result, payload.Model, payload.Attempts, lastError, now)
	return false, err
}
