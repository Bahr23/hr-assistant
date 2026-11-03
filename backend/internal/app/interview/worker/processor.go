package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"hr-assistant/backend/internal/app/answerevaluation"
	"hr-assistant/backend/internal/app/candidatereport"
	"hr-assistant/backend/internal/core/messaging"
	report "hr-assistant/shared/candidatereport"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Processor struct {
	pool       *pgxpool.Pool
	shardCount int
	now        func() time.Time
}

func NewProcessor(pool *pgxpool.Pool, shardCount int) *Processor {
	return &Processor{pool: pool, shardCount: shardCount, now: func() time.Time { return time.Now().UTC() }}
}

// Process returns permanent=true only for a poison message that must go to DLQ.
func (p *Processor) Process(ctx context.Context, envelope messaging.Envelope, shard int) (permanent bool, err error) {
	if envelope.MessageID == uuid.Nil || envelope.CorrelationID == uuid.Nil || envelope.SchemaVersion != 1 {
		return true, fmt.Errorf("invalid message envelope")
	}
	if messaging.Shard(envelope.PartitionKey, p.shardCount) != shard {
		return true, fmt.Errorf("partition key does not belong to shard %d", shard)
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

	switch envelope.Type {
	case StartRequested:
		var payload StartPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		if err := validateStart(payload); err != nil {
			return true, err
		}
		if err := upsertTelegramCandidate(ctx, tx, payload.CandidateIdentity, p.now()); err != nil {
			return false, err
		}
		if err := p.handleStart(ctx, tx, envelope, payload, false); err != nil {
			return false, err
		}
	case SwitchRequested:
		var payload StartPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		if err := validateStart(payload); err != nil {
			return true, err
		}
		if err := upsertTelegramCandidate(ctx, tx, payload.CandidateIdentity, p.now()); err != nil {
			return false, err
		}
		if err := p.handleStart(ctx, tx, envelope, payload, true); err != nil {
			return false, err
		}
	case ContinueRequested:
		var payload CandidateIdentity
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		if err := validateIdentity(payload); err != nil {
			return true, err
		}
		if err := upsertTelegramCandidate(ctx, tx, payload, p.now()); err != nil {
			return false, err
		}
		if err := p.handleContinue(ctx, tx, envelope, payload); err != nil {
			return false, err
		}
	case CancelRequested:
		var payload CandidateIdentity
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		if err := validateIdentity(payload); err != nil {
			return true, err
		}
		if err := upsertTelegramCandidate(ctx, tx, payload, p.now()); err != nil {
			return false, err
		}
		if err := p.handleCancel(ctx, tx, envelope, payload); err != nil {
			return false, err
		}
	case AnswerReceived:
		var payload AnswerPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		if err := validateAnswer(payload); err != nil {
			return true, err
		}
		if err := upsertTelegramCandidate(ctx, tx, payload.CandidateIdentity, p.now()); err != nil {
			return false, err
		}
		if err := p.handleAnswer(ctx, tx, envelope, payload); err != nil {
			return false, err
		}
	case DeliverySucceeded:
		var payload DeliverySucceededPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		if payload.DeliveryID == uuid.Nil || payload.TelegramMessageID == 0 {
			return true, fmt.Errorf("invalid successful delivery payload")
		}
		_, err = tx.Exec(ctx, `
UPDATE telegram_deliveries
SET status = 'sent', telegram_message_id = $2, sent_at = $3, updated_at = $3
WHERE id = $1 AND telegram_chat_id = $4 AND status = 'pending'`,
			payload.DeliveryID, payload.TelegramMessageID, p.now(), envelope.PartitionKey)
		if err != nil {
			return false, err
		}
	case DeliveryFailed:
		var payload DeliveryFailedPayload
		if err := decodePayload(envelope.Payload, &payload); err != nil {
			return true, err
		}
		if payload.DeliveryID == uuid.Nil || strings.TrimSpace(payload.Code) == "" {
			return true, fmt.Errorf("invalid failed delivery payload")
		}
		_, err = tx.Exec(ctx, `
UPDATE telegram_deliveries
SET status = 'failed', failure_code = $2, failure_message = $3, updated_at = $4
WHERE id = $1 AND telegram_chat_id = $5 AND status = 'pending'`,
			payload.DeliveryID, payload.Code, payload.Message, p.now(), envelope.PartitionKey)
		if err != nil {
			return false, err
		}
	default:
		return true, fmt.Errorf("unsupported message type %q", envelope.Type)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return false, nil
}

// ExpireNext closes one overdue session and queues the notification in the same transaction.
func (p *Processor) ExpireNext(ctx context.Context) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	now := p.now()
	var sessionID uuid.UUID
	var chatID int64
	err = tx.QueryRow(ctx, `
SELECT id, telegram_chat_id
FROM interview_sessions
WHERE status = 'active' AND expires_at <= $1
ORDER BY expires_at, id
FOR UPDATE SKIP LOCKED
LIMIT 1`, now).Scan(&sessionID, &chatID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := expireSession(ctx, tx, sessionID, now); err != nil {
		return false, err
	}
	parent := messaging.Envelope{CorrelationID: uuid.New()}
	if err := p.enqueueSystem(ctx, tx, parent, chatID, &sessionID,
		"Время прохождения интервью истекло. Ответы больше не принимаются."); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

type activeSession struct {
	ID              uuid.UUID
	VacancyID       uuid.UUID
	CurrentPosition int
	Status          string
	ExpiresAt       time.Time
}

type vacancySnapshot struct {
	Title                    string
	Description              *string
	Criteria                 []report.Criterion
	ID                       uuid.UUID
	InterviewDurationMinutes int
	Questions                []snapshotQuestion
}

type snapshotQuestion struct {
	EvaluationGuide *string
	ID              uuid.UUID
	Text            string
	AnswerType      string
	Position        int
}

func (p *Processor) handleStart(
	ctx context.Context,
	tx pgx.Tx,
	envelope messaging.Envelope,
	payload StartPayload,
	confirmedSwitch bool,
) error {
	if err := validateStart(payload); err != nil {
		return err
	}
	vacancy, err := loadVacancySnapshot(ctx, tx, payload.PublicToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, nil,
			"Вакансия не найдена или больше не активна.")
	}
	if err != nil {
		return err
	}

	previous, attempted, err := lockVacancyAttempt(ctx, tx, payload.TelegramUserID, vacancy.ID)
	if err != nil {
		return err
	}
	if attempted {
		if previous.Status == "active" && !sessionExpired(previous, p.now()) {
			return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, &previous.ID,
				"Вы уже проходите интервью по этой вакансии. Ответьте на текущий вопрос, чтобы продолжить.")
		}
		if previous.Status == "active" {
			if err := expireSession(ctx, tx, previous.ID, p.now()); err != nil {
				return err
			}
			return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, &previous.ID,
				"Время прохождения интервью истекло. Повторная попытка для этой вакансии недоступна.")
		}
		return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, &previous.ID,
			"Вы уже проходили интервью по этой вакансии. Повторное прохождение недоступно.")
	}

	active, found, err := lockActiveSession(ctx, tx, payload.TelegramUserID)
	if err != nil {
		return err
	}
	if found && sessionExpired(active, p.now()) {
		if err := expireSession(ctx, tx, active.ID, p.now()); err != nil {
			return err
		}
		found = false
	}
	if found && !confirmedSwitch {
		return p.enqueueConflict(ctx, tx, envelope, payload.TelegramChatID, active.ID, payload.PublicToken)
	}
	if found {
		now := p.now()
		if _, err := tx.Exec(ctx, `
UPDATE interview_sessions
SET status = 'cancelled', cancelled_at = $2, updated_at = $2
WHERE id = $1`, active.ID, now); err != nil {
			return err
		}
	}
	created, err := p.createSession(ctx, tx, payload, vacancy)
	if err != nil {
		return err
	}
	return p.enqueueQuestion(ctx, tx, envelope, payload.TelegramChatID, created)
}

func (p *Processor) createSession(
	ctx context.Context,
	tx pgx.Tx,
	payload StartPayload,
	vacancy vacancySnapshot,
) (activeSession, error) {
	now := p.now()
	expiresAt := now.Add(time.Duration(vacancy.InterviewDurationMinutes) * time.Minute)
	session := activeSession{
		ID: uuid.New(), VacancyID: vacancy.ID, CurrentPosition: 0,
		Status: "active", ExpiresAt: expiresAt,
	}
	claim, err := tx.Exec(ctx, `
INSERT INTO interview_attempt_claims (
    vacancy_id, telegram_user_id, first_session_id, created_at
) VALUES ($1, $2, $3, $4)
ON CONFLICT (vacancy_id, telegram_user_id) DO NOTHING`,
		vacancy.ID, payload.TelegramUserID, session.ID, now)
	if err != nil {
		return activeSession{}, err
	}
	if claim.RowsAffected() == 0 {
		return activeSession{}, fmt.Errorf("interview attempt already claimed")
	}
	_, err = tx.Exec(ctx, `
INSERT INTO interview_sessions (
    id, vacancy_id, telegram_user_id, telegram_chat_id, status,
    current_question_position, expires_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, 'active', 0, $5, $6, $6)`,
		session.ID, vacancy.ID, payload.TelegramUserID, payload.TelegramChatID, expiresAt, now)
	if err != nil {
		return activeSession{}, err
	}
	criteriaJSON, err := json.Marshal(vacancy.Criteria)
	if err != nil {
		return activeSession{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE interview_sessions SET vacancy_title=$2,vacancy_description=$3,evaluation_criteria=$4 WHERE id=$1`, session.ID, vacancy.Title, vacancy.Description, criteriaJSON); err != nil {
		return activeSession{}, err
	}
	for _, question := range vacancy.Questions {
		_, err := tx.Exec(ctx, `
INSERT INTO interview_session_questions (
    id, session_id, source_question_id, text, answer_type, position, created_at, evaluation_guide
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			uuid.New(), session.ID, question.ID, question.Text, question.AnswerType, question.Position, now, question.EvaluationGuide)
		if err != nil {
			return activeSession{}, err
		}
	}
	return session, nil
}

func (p *Processor) handleContinue(
	ctx context.Context,
	tx pgx.Tx,
	envelope messaging.Envelope,
	payload CandidateIdentity,
) error {
	active, found, err := lockActiveSession(ctx, tx, payload.TelegramUserID)
	if err != nil {
		return err
	}
	if !found {
		return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, nil,
			"Сейчас нет активного интервью.")
	}
	if sessionExpired(active, p.now()) {
		if err := expireSession(ctx, tx, active.ID, p.now()); err != nil {
			return err
		}
		return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, &active.ID,
			"Время прохождения интервью истекло.")
	}
	return p.enqueueQuestion(ctx, tx, envelope, payload.TelegramChatID, active)
}

func (p *Processor) handleCancel(
	ctx context.Context,
	tx pgx.Tx,
	envelope messaging.Envelope,
	payload CandidateIdentity,
) error {
	active, found, err := lockActiveSession(ctx, tx, payload.TelegramUserID)
	if err != nil {
		return err
	}
	if !found {
		return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, nil,
			"Сейчас нет активного интервью.")
	}
	if sessionExpired(active, p.now()) {
		if err := expireSession(ctx, tx, active.ID, p.now()); err != nil {
			return err
		}
		return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, &active.ID,
			"Время прохождения интервью истекло.")
	}
	now := p.now()
	if _, err := tx.Exec(ctx, `
UPDATE interview_sessions
SET status = 'cancelled', cancelled_at = $2, updated_at = $2
WHERE id = $1`, active.ID, now); err != nil {
		return err
	}
	return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, &active.ID,
		"Интервью отменено.")
}

func (p *Processor) handleAnswer(
	ctx context.Context,
	tx pgx.Tx,
	envelope messaging.Envelope,
	payload AnswerPayload,
) error {
	active, found, err := lockActiveSession(ctx, tx, payload.TelegramUserID)
	if err != nil {
		return err
	}
	if !found {
		return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, nil,
			"Сейчас нет активного интервью. Откройте ссылку вакансии, чтобы начать.")
	}
	if sessionExpired(active, p.now()) {
		if err := expireSession(ctx, tx, active.ID, p.now()); err != nil {
			return err
		}
		return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, &active.ID,
			"Время прохождения интервью истекло. Ответ не сохранён.")
	}
	question, total, err := loadSessionQuestion(ctx, tx, active.ID, active.CurrentPosition)
	if err != nil {
		return err
	}
	if question.AnswerType != payload.AnswerType {
		hint := "Ответьте на этот вопрос текстовым сообщением."
		if question.AnswerType == "voice" {
			hint = "Ответьте на этот вопрос голосовым сообщением."
		}
		return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, &active.ID, hint)
	}

	now := p.now()
	var textAnswer, voiceFileID *string
	var voiceMimeType *string
	var voiceFileSize *int64
	var voiceDuration *int
	if payload.AnswerType == "text" {
		value := strings.TrimSpace(payload.Text)
		textAnswer = &value
	} else {
		voiceFileID = &payload.VoiceFileID
		if payload.VoiceMimeType != "" {
			voiceMimeType = &payload.VoiceMimeType
		}
		if payload.VoiceFileSize > 0 {
			voiceFileSize = &payload.VoiceFileSize
		}
		if payload.VoiceDuration > 0 {
			voiceDuration = &payload.VoiceDuration
		}
	}
	command, err := tx.Exec(ctx, `
INSERT INTO interview_answers (
    id, session_id, question_position, telegram_message_id, answer_type,
    text_answer, telegram_voice_file_id, telegram_voice_mime_type,
    telegram_voice_file_size, telegram_voice_duration_seconds, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
ON CONFLICT DO NOTHING`,
		uuid.New(), active.ID, active.CurrentPosition, payload.TelegramMessageID,
		payload.AnswerType, textAnswer, voiceFileID, voiceMimeType,
		voiceFileSize, voiceDuration, now)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return nil
	}

	nextPosition := active.CurrentPosition + 1
	if nextPosition >= total {
		if _, err := tx.Exec(ctx, `
UPDATE interview_sessions
SET status = 'completed', completed_at = $2, updated_at = $2
WHERE id = $1`, active.ID, now); err != nil {
			return err
		}
		if err := p.enqueueInterviewCompleted(ctx, tx, envelope, payload.TelegramChatID, active, now); err != nil {
			return err
		}
		return p.enqueueSystem(ctx, tx, envelope, payload.TelegramChatID, &active.ID,
			"Спасибо! Интервью завершено, ваши ответы сохранены. В ближайшее время вам придет фидбек, основанный на ваших ответах.")
	}
	if _, err := tx.Exec(ctx, `
UPDATE interview_sessions
SET current_question_position = $2, updated_at = $3
WHERE id = $1`, active.ID, nextPosition, now); err != nil {
		return err
	}
	active.CurrentPosition = nextPosition
	return p.enqueueQuestion(ctx, tx, envelope, payload.TelegramChatID, active)
}

func (p *Processor) enqueueInterviewCompleted(
	ctx context.Context,
	tx pgx.Tx,
	parent messaging.Envelope,
	chatID int64,
	session activeSession,
	now time.Time,
) error {
	if _, err := tx.Exec(ctx, `
INSERT INTO interview_answer_transcriptions (answer_id, status, created_at, updated_at)
SELECT id, 'pending', $2, $2
FROM interview_answers
WHERE session_id = $1 AND answer_type = 'voice'
ON CONFLICT (answer_id) DO NOTHING`, session.ID, now); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
SELECT id, telegram_voice_file_id, COALESCE(telegram_voice_mime_type, ''),
       COALESCE(telegram_voice_file_size, 0),
       COALESCE(telegram_voice_duration_seconds, 0)
FROM interview_answers
WHERE session_id = $1 AND answer_type = 'voice'
ORDER BY question_position`, session.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	answers := make([]CompletedVoiceAnswer, 0)
	for rows.Next() {
		var answer CompletedVoiceAnswer
		if err := rows.Scan(
			&answer.AnswerID, &answer.TelegramFileID, &answer.MimeType,
			&answer.FileSize, &answer.Duration,
		); err != nil {
			return err
		}
		answers = append(answers, answer)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var language string
	if err := tx.QueryRow(ctx,
		"SELECT interview_language FROM vacancies WHERE id = $1", session.VacancyID,
	).Scan(&language); err != nil {
		return err
	}
	if err := answerevaluation.ScheduleReady(ctx, tx, session.ID, now); err != nil {
		return err
	}
	if err := candidatereport.Ensure(ctx, tx, session.ID, now); err != nil {
		return err
	}
	payload := InterviewCompletedPayload{
		SessionID: session.ID, VacancyID: session.VacancyID,
		TelegramChatID: chatID, Language: language, VoiceAnswers: answers,
	}
	return enqueueOutboxEnvelope(ctx, tx, parent, messaging.InterviewExchange,
		InterviewCompleted, InterviewCompleted, chatID, payload, now)
}

func loadVacancySnapshot(ctx context.Context, tx pgx.Tx, publicToken string) (vacancySnapshot, error) {
	var result vacancySnapshot
	err := tx.QueryRow(ctx, `
SELECT id, interview_duration_minutes, title, description
FROM vacancies
WHERE public_token = $1
  AND status = 'published'
  AND (expires_at IS NULL OR expires_at > now()) FOR SHARE`, publicToken).Scan(
		&result.ID, &result.InterviewDurationMinutes, &result.Title, &result.Description,
	)
	if err != nil {
		return vacancySnapshot{}, err
	}
	rows, err := tx.Query(ctx, `
SELECT id, text, answer_type, position, evaluation_guide
FROM questions
WHERE vacancy_id = $1
ORDER BY position`, result.ID)
	if err != nil {
		return vacancySnapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var question snapshotQuestion
		if err := rows.Scan(&question.ID, &question.Text, &question.AnswerType, &question.Position, &question.EvaluationGuide); err != nil {
			return vacancySnapshot{}, err
		}
		result.Questions = append(result.Questions, question)
	}
	if err := rows.Err(); err != nil {
		return vacancySnapshot{}, err
	}
	rows.Close()
	criterionRows, err := tx.Query(ctx, `SELECT id,title,evaluation_guide,position,source_template_id FROM vacancy_evaluation_criteria WHERE vacancy_id=$1 ORDER BY position`, result.ID)
	if err != nil {
		return vacancySnapshot{}, err
	}
	result.Criteria = []report.Criterion{}
	for criterionRows.Next() {
		var c report.Criterion
		if err := criterionRows.Scan(&c.ID, &c.Title, &c.EvaluationGuide, &c.Position, &c.SourceTemplateID); err != nil {
			criterionRows.Close()
			return vacancySnapshot{}, err
		}
		result.Criteria = append(result.Criteria, c)
	}
	criterionRows.Close()
	if err := criterionRows.Err(); err != nil {
		return vacancySnapshot{}, err
	}
	if len(result.Questions) == 0 {
		return vacancySnapshot{}, pgx.ErrNoRows
	}
	return result, nil
}

func lockActiveSession(ctx context.Context, tx pgx.Tx, telegramUserID int64) (activeSession, bool, error) {
	var result activeSession
	err := tx.QueryRow(ctx, `
SELECT id, vacancy_id, current_question_position, status, expires_at
FROM interview_sessions
WHERE telegram_user_id = $1 AND status = 'active'
FOR UPDATE`, telegramUserID).Scan(
		&result.ID, &result.VacancyID, &result.CurrentPosition, &result.Status, &result.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return activeSession{}, false, nil
	}
	return result, err == nil, err
}

func lockVacancyAttempt(
	ctx context.Context,
	tx pgx.Tx,
	telegramUserID int64,
	vacancyID uuid.UUID,
) (activeSession, bool, error) {
	var result activeSession
	err := tx.QueryRow(ctx, `
SELECT sessions.id, sessions.vacancy_id, sessions.current_question_position,
       sessions.status, sessions.expires_at
FROM interview_sessions AS sessions
JOIN interview_attempt_claims AS claims
  ON claims.first_session_id = sessions.id
WHERE claims.telegram_user_id = $1 AND claims.vacancy_id = $2
FOR UPDATE OF sessions`, telegramUserID, vacancyID).Scan(
		&result.ID, &result.VacancyID, &result.CurrentPosition, &result.Status, &result.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return activeSession{}, false, nil
	}
	return result, err == nil, err
}

func sessionExpired(session activeSession, now time.Time) bool {
	return !session.ExpiresAt.After(now)
}

func expireSession(ctx context.Context, tx pgx.Tx, sessionID uuid.UUID, now time.Time) error {
	_, err := tx.Exec(ctx, `
UPDATE interview_sessions
SET status = 'expired', expired_at = $2, updated_at = $2
WHERE id = $1 AND status = 'active'`, sessionID, now)
	return err
}

func loadSessionQuestion(
	ctx context.Context,
	tx pgx.Tx,
	sessionID uuid.UUID,
	position int,
) (snapshotQuestion, int, error) {
	var result snapshotQuestion
	var total int
	err := tx.QueryRow(ctx, `
SELECT id, text, answer_type, position,
       (SELECT count(*)::integer FROM interview_session_questions WHERE session_id = $1)
FROM interview_session_questions
WHERE session_id = $1 AND position = $2`, sessionID, position).Scan(
		&result.ID, &result.Text, &result.AnswerType, &result.Position, &total,
	)
	return result, total, err
}

func (p *Processor) enqueueQuestion(
	ctx context.Context,
	tx pgx.Tx,
	envelope messaging.Envelope,
	chatID int64,
	session activeSession,
) error {
	question, total, err := loadSessionQuestion(ctx, tx, session.ID, session.CurrentPosition)
	if err != nil {
		return err
	}
	return p.enqueueSend(ctx, tx, envelope, chatID, &session.ID, &question.Position,
		"question", TelegramQuestion, question.Text, question.AnswerType,
		question.Position+1, total, nil)
}

func (p *Processor) enqueueSystem(
	ctx context.Context,
	tx pgx.Tx,
	envelope messaging.Envelope,
	chatID int64,
	sessionID *uuid.UUID,
	text string,
) error {
	return p.enqueueSend(ctx, tx, envelope, chatID, sessionID, nil,
		"system", TelegramSystem, text, "", 0, 0, nil)
}

func (p *Processor) enqueueConflict(
	ctx context.Context,
	tx pgx.Tx,
	envelope messaging.Envelope,
	chatID int64,
	sessionID uuid.UUID,
	publicToken string,
) error {
	buttons := [][]InlineButton{
		{{Text: "Продолжить текущее", CallbackData: "interview:continue"}},
		{{Text: "Отменить и начать новое", CallbackData: "interview:switch:" + publicToken}},
	}
	return p.enqueueSend(ctx, tx, envelope, chatID, &sessionID, nil,
		"session_conflict", TelegramConflict,
		"У вас уже есть активное интервью. Продолжить его или начать новое?", "", 0, 0, buttons)
}

func (p *Processor) enqueueSend(
	ctx context.Context,
	tx pgx.Tx,
	parent messaging.Envelope,
	chatID int64,
	sessionID *uuid.UUID,
	questionPosition *int,
	kind, messageType, text, answerType string,
	questionNumber, questionTotal int,
	buttons [][]InlineButton,
) error {
	deliveryID := uuid.New()
	now := p.now()
	_, err := tx.Exec(ctx, `
INSERT INTO telegram_deliveries (
    id, session_id, question_position, telegram_chat_id, kind, status,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, 'pending', $6, $6)`,
		deliveryID, sessionID, questionPosition, chatID, kind, now)
	if err != nil {
		return err
	}
	payload := TelegramSendPayload{
		DeliveryID: deliveryID, SessionID: sessionID, ChatID: chatID, Text: text,
		AnswerType: answerType, QuestionNumber: questionNumber, QuestionTotal: questionTotal,
		Buttons: buttons,
	}
	outgoing, err := messaging.NewEnvelope(messageType, parent.CorrelationID, chatID, payload)
	if err != nil {
		return err
	}
	outgoing.MessageID = deliveryID
	body, err := json.Marshal(outgoing)
	if err != nil {
		return err
	}
	shard := messaging.Shard(chatID, p.shardCount)
	routingKey := fmt.Sprintf("telegram.%d.%s", shard, strings.TrimPrefix(messageType, "telegram."))
	_, err = tx.Exec(ctx, `
INSERT INTO outbox_events (id, exchange_name, routing_key, body, created_at)
VALUES ($1, $2, $3, $4, $5)`,
		uuid.New(), messaging.TelegramExchange, routingKey, body, now)
	return err
}

func enqueueOutboxEnvelope(
	ctx context.Context,
	tx pgx.Tx,
	parent messaging.Envelope,
	exchange, routingKey, messageType string,
	partitionKey int64,
	payload any,
	now time.Time,
) error {
	outgoing, err := messaging.NewEnvelope(messageType, parent.CorrelationID, partitionKey, payload)
	if err != nil {
		return err
	}
	body, err := json.Marshal(outgoing)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO outbox_events (id, exchange_name, routing_key, body, created_at)
VALUES ($1, $2, $3, $4, $5)`, uuid.New(), exchange, routingKey, body, now)
	return err
}

func decodePayload(data json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode payload: trailing JSON data")
	}
	return nil
}

func validateIdentity(identity CandidateIdentity) error {
	if identity.TelegramUserID <= 0 || identity.TelegramChatID != identity.TelegramUserID ||
		strings.TrimSpace(identity.FirstName) == "" {
		return fmt.Errorf("invalid Telegram identity")
	}
	return nil
}

func upsertTelegramCandidate(
	ctx context.Context,
	tx pgx.Tx,
	identity CandidateIdentity,
	now time.Time,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO telegram_candidates (
    telegram_user_id, telegram_chat_id, username, first_name, last_name,
    is_premium, language_code,
    first_seen_at, last_seen_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)
ON CONFLICT (telegram_user_id) DO UPDATE
SET telegram_chat_id = EXCLUDED.telegram_chat_id,
    username = EXCLUDED.username,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    is_premium = EXCLUDED.is_premium,
    language_code = EXCLUDED.language_code,
    last_seen_at = EXCLUDED.last_seen_at`,
		identity.TelegramUserID, identity.TelegramChatID,
		identity.Username, identity.FirstName, identity.LastName,
		identity.IsPremium, identity.LanguageCode, now,
	)
	return err
}

func validateStart(payload StartPayload) error {
	if err := validateIdentity(payload.CandidateIdentity); err != nil {
		return err
	}
	if len(payload.PublicToken) == 0 || len(payload.PublicToken) > 64 {
		return fmt.Errorf("invalid public token")
	}
	for _, char := range payload.PublicToken {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return fmt.Errorf("invalid public token")
		}
	}
	return nil
}

func validateAnswer(payload AnswerPayload) error {
	if err := validateIdentity(payload.CandidateIdentity); err != nil {
		return err
	}
	if payload.TelegramMessageID <= 0 {
		return fmt.Errorf("invalid Telegram message id")
	}
	switch payload.AnswerType {
	case "text":
		if strings.TrimSpace(payload.Text) == "" || payload.VoiceFileID != "" {
			return fmt.Errorf("invalid text answer")
		}
	case "voice":
		if payload.VoiceFileID == "" || payload.Text != "" ||
			payload.VoiceFileSize < 0 || payload.VoiceDuration < 0 {
			return fmt.Errorf("invalid voice answer")
		}
	default:
		return fmt.Errorf("invalid answer type")
	}
	return nil
}
