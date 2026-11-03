package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"hr-assistant/openrouter-worker/internal/core/messaging"
	report "hr-assistant/shared/candidatereport"
)

func (r *Repository) IngestReport(ctx context.Context, envelope messaging.Envelope, model string) (int, error) {
	if envelope.Type != report.Requested || envelope.SchemaVersion != 1 || envelope.MessageID == uuid.Nil ||
		envelope.CorrelationID == uuid.Nil || envelope.PartitionKey <= 0 || strings.TrimSpace(model) == "" {
		return 0, fmt.Errorf("%w: invalid evaluation envelope", ErrInvalidEvent)
	}
	var request report.Request
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return 0, fmt.Errorf("%w: invalid evaluation payload", ErrInvalidEvent)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return 0, fmt.Errorf("%w: trailing evaluation payload", ErrInvalidEvent)
	}
	if request.SessionID == uuid.Nil || request.Version < 1 ||
		request.PromptVersion == "" || request.SchemaVersion == "" || !request.Input.Valid() {
		return 0, fmt.Errorf("%w: invalid evaluation input", ErrInvalidEvent)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	command, err := tx.Exec(ctx, `INSERT INTO inbox_events (message_id, message_type) VALUES ($1, $2)
ON CONFLICT (message_id) DO NOTHING`, envelope.MessageID, envelope.Type)
	if err != nil {
		return 0, err
	}
	if command.RowsAffected() == 0 {
		return 0, tx.Commit(ctx)
	}
	// Serialize against terminal receipt creation and other deliveries for this session.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, request.SessionID.String()); err != nil {
		return 0, err
	}
	actionID := uuid.New()
	command, err = tx.Exec(ctx, `
INSERT INTO action_jobs (id, source_message_id, action_type, subject_id, session_id, partition_key, analysis_version, status)
SELECT $1, $2, $3, $4, $5, $6, $7, 'pending'
WHERE NOT EXISTS (SELECT 1 FROM completed_candidate_reports WHERE session_id = $4 AND version = $7)
ON CONFLICT (action_type, subject_id, analysis_version) DO NOTHING`,
		actionID, envelope.MessageID, report.Action, request.SessionID, request.SessionID, envelope.PartitionKey, request.Version)
	if err != nil {
		return 0, err
	}
	if command.RowsAffected() == 0 {
		return 0, tx.Commit(ctx)
	}
	input, err := json.Marshal(request.Input)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO candidate_report_jobs (action_id, input, model, prompt_version, schema_version)
VALUES ($1, $2, $3, $4, $5)`, actionID, input, model, request.PromptVersion, request.SchemaVersion)
	if err != nil {
		return 0, err
	}
	return 1, tx.Commit(ctx)
}
