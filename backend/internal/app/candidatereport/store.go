// Package candidatereport owns report readiness, immutable inputs and version history.
package candidatereport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"hr-assistant/backend/internal/app/answerevaluation"
	"hr-assistant/backend/internal/app/candidatefeedback"
	"hr-assistant/backend/internal/core/messaging"
	report "hr-assistant/shared/candidatereport"
	"strings"
	"time"
)

var ErrNotRetryable = errors.New("report is not retryable")

// Ensure creates the initial waiting version only when explicitly called on completion.
func Ensure(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, now time.Time) error {
	if err := lockSession(ctx, tx, sessionID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO candidate_reports(session_id,version,status,prompt_version,schema_version,total_criteria,created_at,updated_at)
 SELECT id,1,'pending',$2,$3,jsonb_array_length(evaluation_criteria),$4,$4 FROM interview_sessions WHERE id=$1 AND status='completed'
 ON CONFLICT(session_id,version) DO NOTHING`, sessionID, report.PromptVersion, report.SchemaVersion, now); err != nil {
		return err
	}
	return ScheduleReady(ctx, tx, sessionID, now)
}
func lockSession(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	var locked uuid.UUID
	return tx.QueryRow(ctx, `SELECT id FROM interview_sessions WHERE id=$1 AND status='completed' FOR UPDATE`, id).Scan(&locked)
}

// Serialize readiness checks. The query following this lock sees committed results
// from concurrent answer completions under READ COMMITTED, avoiding missed wakeups.
func ScheduleReady(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, now time.Time) error {
	if err := lockSession(ctx, tx, sessionID); err != nil {
		return err
	}
	var version int
	err := tx.QueryRow(ctx, `SELECT version FROM candidate_reports WHERE session_id=$1 AND status='pending' AND input_snapshot IS NULL ORDER BY version LIMIT 1`, sessionID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var input report.Input
	var criteriaJSON []byte
	var title *string
	var chatID int64
	err = tx.QueryRow(ctx, `SELECT vacancy_title,vacancy_description,evaluation_criteria,telegram_chat_id FROM interview_sessions WHERE id=$1`, sessionID).Scan(&title, &input.VacancyDescription, &criteriaJSON, &chatID)
	if err != nil {
		return err
	}
	input.LegacyContext = title == nil
	if title != nil {
		input.VacancyTitle = *title
	}
	if err = json.Unmarshal(criteriaJSON, &input.Criteria); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT q.position+1,q.text,q.evaluation_guide,
 CASE WHEN q.answer_type='text' THEN a.text_answer WHEN t.status='succeeded' THEN t.transcription_text END,
 CASE WHEN q.answer_type='text' THEN 'text' ELSE 'transcription' END,
 CASE WHEN q.answer_type='text' THEN 'skipped' ELSE COALESCE(t.status,'pending') END,
 CASE WHEN a.id IS NULL THEN 'pending' WHEN q.evaluation_guide IS NULL OR t.status='permanent_failed' THEN 'skipped' ELSE COALESCE(e.status,'pending') END,
 CASE WHEN e.status='succeeded' THEN e.result END
 FROM interview_session_questions q
 LEFT JOIN interview_answers a ON a.session_id=q.session_id AND a.question_position=q.position
 LEFT JOIN interview_answer_transcriptions t ON t.answer_id=a.id
 LEFT JOIN LATERAL(SELECT status,result FROM interview_answer_evaluations WHERE answer_id=a.id ORDER BY version DESC LIMIT 1)e ON true
 WHERE q.session_id=$1 ORDER BY q.position`, sessionID)
	if err != nil {
		return err
	}
	ready := true
	input.Answers = []report.Answer{}
	for rows.Next() {
		var a report.Answer
		var evaluationJSON []byte
		if err := rows.Scan(&a.QuestionNumber, &a.Question, &a.Guide, &a.Text, &a.Source, &a.TranscriptionStatus, &a.EvaluationStatus, &evaluationJSON); err != nil {
			rows.Close()
			return err
		}
		if a.TranscriptionStatus == "pending" || a.EvaluationStatus == "pending" {
			ready = false
		}
		if evaluationJSON != nil {
			if err := json.Unmarshal(evaluationJSON, &a.Evaluation); err != nil {
				rows.Close()
				return err
			}
		}
		input.Answers = append(input.Answers, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if !ready {
		return nil
	}
	if !input.Valid() {
		return fmt.Errorf("invalid candidate report input")
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE candidate_reports SET input_snapshot=$3,updated_at=$4 WHERE session_id=$1 AND version=$2`, sessionID, version, raw, now); err != nil {
		return err
	}
	request := report.Request{SessionID: sessionID, Version: version, PromptVersion: report.PromptVersion, SchemaVersion: report.SchemaVersion, Input: input}
	envelope, err := messaging.NewEnvelope(report.Requested, sessionID, chatID, request)
	if err != nil {
		return err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox_events(id,exchange_name,routing_key,body,created_at) VALUES($1,$2,$3,$4,$5)`, envelope.MessageID, messaging.InterviewExchange, report.Requested, body, now)
	return err
}

// Retry uses the version observed by HR as its idempotency key, including 0 for legacy sessions.
func Retry(ctx context.Context, tx pgx.Tx, hrID, sessionID uuid.UUID, observed int, now time.Time) (int, error) {
	var locked uuid.UUID
	err := tx.QueryRow(ctx, `SELECT s.id FROM interview_sessions s JOIN vacancies v ON v.id=s.vacancy_id
 WHERE s.id=$1 AND s.status='completed' AND v.owner_user_id=$2 FOR UPDATE OF s`, sessionID, hrID).Scan(&locked)
	if err != nil {
		return 0, err
	}
	var current int
	var status string
	err = tx.QueryRow(ctx, `SELECT version,status FROM candidate_reports WHERE session_id=$1 ORDER BY version DESC LIMIT 1`, sessionID).Scan(&current, &status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if current > observed { // Repeated requests cannot create additional versions.
		var exists bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM candidate_reports WHERE session_id=$1 AND version=$2)`, sessionID, observed+1).Scan(&exists)
		if err != nil {
			return 0, err
		}
		if exists {
			return observed + 1, nil
		}
		return 0, ErrNotRetryable
	}
	if current != observed || status == "pending" {
		return 0, ErrNotRetryable
	}
	next := observed + 1
	_, err = tx.Exec(ctx, `INSERT INTO candidate_reports(session_id,version,status,prompt_version,schema_version,total_criteria,created_at,updated_at)
 SELECT id,$2,'pending',$3,$4,jsonb_array_length(evaluation_criteria),$5,$5 FROM interview_sessions WHERE id=$1`, sessionID, next, report.PromptVersion, report.SchemaVersion, now)
	if err != nil {
		return 0, err
	}
	if err := retryFailedEvaluations(ctx, tx, hrID, sessionID, now); err != nil {
		return 0, err
	}
	return next, ScheduleReady(ctx, tx, sessionID, now)
}

// A rebuild repairs failed prerequisites before freezing the next report input.
// The session lock and observed report version keep concurrent rebuilds idempotent.
func retryFailedEvaluations(ctx context.Context, tx pgx.Tx, hrID, sessionID uuid.UUID, now time.Time) error {
	rows, err := tx.Query(ctx, `SELECT a.id,e.version FROM interview_answers a
 JOIN LATERAL(SELECT version,status FROM interview_answer_evaluations WHERE answer_id=a.id ORDER BY version DESC LIMIT 1)e ON true
 WHERE a.session_id=$1 AND e.status='permanent_failed' ORDER BY a.question_position`, sessionID)
	if err != nil {
		return err
	}
	type failed struct {
		id      uuid.UUID
		version int
	}
	var answers []failed
	for rows.Next() {
		var answer failed
		if err := rows.Scan(&answer.id, &answer.version); err != nil {
			rows.Close()
			return err
		}
		answers = append(answers, answer)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, answer := range answers {
		if _, err := answerevaluation.Retry(ctx, tx, hrID, sessionID, answer.id, answer.version, now); err != nil {
			return err
		}
	}
	return nil
}

func SaveResult(ctx context.Context, tx pgx.Tx, eventType string, partition int64, p report.Completion, now time.Time) (bool, error) {
	if p.SessionID == uuid.Nil || p.Version < 1 || p.Attempts < 1 || strings.TrimSpace(p.Model) == "" || len(p.Model) > 255 {
		return true, fmt.Errorf("invalid candidate report completion")
	}
	var status, prompt, schema string
	var inputJSON []byte
	err := tx.QueryRow(ctx, `SELECT r.status,r.prompt_version,r.schema_version,r.input_snapshot FROM candidate_reports r JOIN interview_sessions s ON s.id=r.session_id
 WHERE r.session_id=$1 AND r.version=$2 AND s.telegram_chat_id=$3 AND s.status='completed' FOR UPDATE OF r`, p.SessionID, p.Version, partition).Scan(&status, &prompt, &schema, &inputJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, fmt.Errorf("candidate report target does not exist")
	}
	if err != nil {
		return false, err
	}
	if prompt != p.PromptVersion || schema != p.SchemaVersion || inputJSON == nil {
		return true, fmt.Errorf("candidate report version mismatch")
	}
	if status != "pending" {
		return false, nil
	}
	var input report.Input
	if err = json.Unmarshal(inputJSON, &input); err != nil {
		return false, err
	}
	var raw []byte
	var parsedReport report.Result
	var code *string
	score := report.Score{TotalCriteria: len(input.Criteria)}
	if eventType == report.Succeeded {
		parsed, parseErr := report.Parse(p.Result, input)
		if parseErr != nil {
			status = "permanent_failed"
			c := "invalid_result"
			code = &c
		} else {
			status = "succeeded"
			parsedReport = parsed
			score = parsed.Score()
			raw, err = json.Marshal(parsed)
			if err != nil {
				return false, err
			}
		}
	} else if eventType == report.Failed && p.Code != "" && len(p.Result) == 0 {
		status = "permanent_failed"
		c := "report_failed"
		switch p.Code {
		case "invalid_result", "unsupported_version", "invalid_input", "openrouter_rejected", "openrouter_auth_failed", "openrouter_unavailable", "model_refusal":
			c = p.Code
		}
		code = &c
	} else {
		return true, fmt.Errorf("invalid candidate report result")
	}
	// Usage is a bounded object, never provider error text or credentials.
	var usage map[string]float64
	if len(p.Usage) > 4096 || (len(p.Usage) > 0 && json.Unmarshal(p.Usage, &usage) != nil) {
		return true, fmt.Errorf("invalid report usage")
	}
	_, err = tx.Exec(ctx, `UPDATE candidate_reports SET status=$3,result=$4,overall_score=$5,scored_criteria=$6,total_criteria=$7,
 model=$8,error_code=$9,attempts=$10,usage=$11,updated_at=$12 WHERE session_id=$1 AND version=$2`, p.SessionID, p.Version, status, raw, score.OverallScore, score.ScoredCriteria, score.TotalCriteria, p.Model, code, p.Attempts, []byte(p.Usage), now)
	if err != nil {
		return false, err
	}
	if status == "succeeded" {
		return false, candidatefeedback.Ensure(ctx, tx, p.SessionID, p.Version, partition, input, parsedReport, now)
	}
	return false, nil
}

type View struct {
	Status         string         `json:"status"`
	Version        int            `json:"version"`
	ResultVersion  *int           `json:"result_version"`
	Result         *report.Result `json:"result"`
	OverallScore   *float64       `json:"overall_score"`
	ScoredCriteria int            `json:"scored_criteria"`
	TotalCriteria  int            `json:"total_criteria"`
	ErrorCode      *string        `json:"error_code"`
	Model          *string        `json:"model"`
	PromptVersion  *string        `json:"prompt_version"`
	SchemaVersion  *string        `json:"schema_version"`
}
type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func Load(ctx context.Context, q queryer, id uuid.UUID) (View, error) {
	v := View{Status: "not_started"}
	var raw []byte
	err := q.QueryRow(ctx, `SELECT latest.status,latest.version,latest.error_code,latest.total_criteria,
 ok.version,ok.result,ok.overall_score,COALESCE(ok.scored_criteria,0),ok.model,ok.prompt_version,ok.schema_version
 FROM candidate_reports latest LEFT JOIN LATERAL(SELECT * FROM candidate_reports WHERE session_id=$1 AND status='succeeded' ORDER BY version DESC LIMIT 1)ok ON true
 WHERE latest.session_id=$1 ORDER BY latest.version DESC LIMIT 1`, id).Scan(&v.Status, &v.Version, &v.ErrorCode, &v.TotalCriteria, &v.ResultVersion, &raw, &v.OverallScore, &v.ScoredCriteria, &v.Model, &v.PromptVersion, &v.SchemaVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	if raw != nil {
		err = json.Unmarshal(raw, &v.Result)
	}
	return v, err
}
