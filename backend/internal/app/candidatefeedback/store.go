// Package candidatefeedback schedules one candidate message after a successful report.
package candidatefeedback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"hr-assistant/backend/internal/core/messaging"
	feedback "hr-assistant/shared/candidatefeedback"
	report "hr-assistant/shared/candidatereport"
)

// Ensure shares the report transaction. A rebuild only retries failed feedback;
// pending or successful feedback is never scheduled a second time.
func Ensure(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, version int, chatID int64, input report.Input, result report.Result, now time.Time) error {
	request := feedback.Request{SessionID: sessionID, Version: version, PromptVersion: feedback.PromptVersion,
		SchemaVersion: feedback.SchemaVersion, Input: feedback.FromReport(input, result)}
	raw, err := json.Marshal(request.Input)
	if err != nil {
		return err
	}
	command, err := tx.Exec(ctx, `INSERT INTO candidate_feedback(session_id,report_version,status,input_snapshot,prompt_version,schema_version,created_at,updated_at)
 VALUES($1,$2,'pending',$3,$4,$5,$6,$6)
 ON CONFLICT(session_id) DO UPDATE SET report_version=EXCLUDED.report_version,status='pending',
 input_snapshot=EXCLUDED.input_snapshot,prompt_version=EXCLUDED.prompt_version,schema_version=EXCLUDED.schema_version,
 result=NULL,model=NULL,attempts=0,error_code=NULL,updated_at=EXCLUDED.updated_at
 WHERE candidate_feedback.status='permanent_failed' AND candidate_feedback.report_version < EXCLUDED.report_version`,
		sessionID, version, raw, request.PromptVersion, request.SchemaVersion, now)
	if err != nil || command.RowsAffected() == 0 {
		return err
	}
	envelope, err := messaging.NewEnvelope(feedback.Requested, sessionID, chatID, request)
	if err != nil {
		return err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,exchange_name,routing_key,body,created_at) VALUES($1,$2,$3,$4,$5)`,
		envelope.MessageID, messaging.InterviewExchange, feedback.Requested, body, now)
	return err
}

// SaveResult returns text only for the first valid success. The caller must queue
// its Telegram delivery in this same transaction before committing.
func SaveResult(ctx context.Context, tx pgx.Tx, eventType string, partition int64, p feedback.Completion, now time.Time) (string, bool, error) {
	if p.SessionID == uuid.Nil || p.Version < 1 || p.Attempts < 1 || strings.TrimSpace(p.Model) == "" || len(p.Model) > 255 {
		return "", true, fmt.Errorf("invalid candidate feedback completion")
	}
	var status, prompt, schema string
	var version int
	err := tx.QueryRow(ctx, `SELECT f.status,f.report_version,f.prompt_version,f.schema_version FROM candidate_feedback f
 JOIN interview_sessions s ON s.id=f.session_id
 WHERE f.session_id=$1 AND s.telegram_chat_id=$2 AND s.status='completed' FOR UPDATE OF f`, p.SessionID, partition).
		Scan(&status, &version, &prompt, &schema)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", true, fmt.Errorf("candidate feedback target does not exist")
	}
	if err != nil {
		return "", false, err
	}
	if p.Version < version { // Late completion from an earlier failed generation.
		return "", false, nil
	}
	if p.Version != version || p.PromptVersion != prompt || p.SchemaVersion != schema {
		return "", true, fmt.Errorf("candidate feedback version mismatch")
	}
	if status != "pending" {
		return "", false, nil
	}
	var raw []byte
	var code *string
	var text string
	if eventType == feedback.Succeeded {
		result, parseErr := feedback.Parse(p.Result)
		if parseErr != nil {
			status = "permanent_failed"
			value := "invalid_result"
			code = &value
		} else {
			status, text = "succeeded", result.Text()
			raw, err = json.Marshal(result)
			if err != nil {
				return "", false, err
			}
		}
	} else if eventType == feedback.Failed && p.Code != "" && len(p.Result) == 0 {
		status = "permanent_failed"
		value := "feedback_failed"
		switch p.Code {
		case "invalid_result", "unsupported_version", "invalid_input", "openrouter_rejected", "openrouter_auth_failed", "openrouter_unavailable", "model_refusal":
			value = p.Code
		}
		code = &value
	} else {
		return "", true, fmt.Errorf("invalid candidate feedback result")
	}
	_, err = tx.Exec(ctx, `UPDATE candidate_feedback SET status=$2,result=$3,model=$4,attempts=$5,error_code=$6,updated_at=$7 WHERE session_id=$1`,
		p.SessionID, status, raw, p.Model, p.Attempts, code, now)
	return text, false, err
}
