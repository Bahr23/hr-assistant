package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"hr-assistant/backend/internal/core/messaging"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestProcessorRunsInterviewInOrderAndDeduplicatesMessages(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	userID := uuid.New()
	vacancyID := uuid.New()
	publicToken := "test_" + uuid.NewString()
	telegramUserID := time.Now().UnixNano()
	telegramChatID := telegramUserID
	correlationID := uuid.New()
	messageIDs := make([]uuid.UUID, 0, 3)
	_, err = pool.Exec(ctx, `
INSERT INTO users (id, email, password_hash)
VALUES ($1, $2, 'unused')`, userID, uuid.NewString()+"@example.test")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO vacancies (
    id, owner_user_id, title, interview_language, welcome_message,
    status, public_token
) VALUES ($1, $2, 'Test', 'ru', 'Welcome', 'published', $3)`,
		vacancyID, userID, publicToken)
	require.NoError(t, err)
	for position, answerType := range []string{"text", "voice"} {
		_, err = pool.Exec(ctx, `
INSERT INTO questions (id, vacancy_id, text, answer_type, position)
VALUES ($1, $2, $3, $4, $5)`,
			uuid.New(), vacancyID, "Question", answerType, position)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM outbox_events WHERE body->>'correlation_id' = $1", correlationID.String())
		_, _ = pool.Exec(ctx, "DELETE FROM telegram_deliveries WHERE telegram_chat_id = $1", telegramChatID)
		_, _ = pool.Exec(ctx, "DELETE FROM interview_attempt_claims WHERE telegram_user_id = $1", telegramUserID)
		_, _ = pool.Exec(ctx, "DELETE FROM interview_sessions WHERE telegram_user_id = $1", telegramUserID)
		_, _ = pool.Exec(ctx, "DELETE FROM telegram_candidates WHERE telegram_user_id = $1", telegramUserID)
		for _, messageID := range messageIDs {
			_, _ = pool.Exec(ctx, "DELETE FROM inbox_messages WHERE message_id = $1", messageID)
		}
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	processor := NewProcessor(pool, 8)
	identity := candidateIdentity(telegramUserID)
	start := testEnvelope(t, StartRequested, correlationID, telegramChatID, StartPayload{
		CandidateIdentity: identity,
		PublicToken:       publicToken,
	})
	messageIDs = append(messageIDs, start.MessageID)
	permanent, err := processor.Process(ctx, start, messaging.Shard(telegramChatID, 8))
	require.NoError(t, err)
	require.False(t, permanent)

	// RabbitMQ may deliver the same envelope again before ACK; inbox makes it a no-op.
	permanent, err = processor.Process(ctx, start, messaging.Shard(telegramChatID, 8))
	require.NoError(t, err)
	require.False(t, permanent)

	var sessionID uuid.UUID
	var currentPosition int
	var status string
	require.NoError(t, pool.QueryRow(ctx, `
SELECT id, current_question_position, status
FROM interview_sessions
WHERE telegram_user_id = $1`, telegramUserID).Scan(&sessionID, &currentPosition, &status))
	require.Equal(t, 0, currentPosition)
	require.Equal(t, "active", status)

	// Starting the same vacancy again must not create another attempt or resend the question.
	repeatedStart := testEnvelope(t, StartRequested, correlationID, telegramChatID, StartPayload{
		CandidateIdentity: identity,
		PublicToken:       publicToken,
	})
	messageIDs = append(messageIDs, repeatedStart.MessageID)
	_, err = processor.Process(ctx, repeatedStart, messaging.Shard(telegramChatID, 8))
	require.NoError(t, err)
	var attemptCount int
	require.NoError(t, pool.QueryRow(ctx, `
SELECT count(*) FROM interview_sessions
WHERE vacancy_id = $1 AND telegram_user_id = $2`, vacancyID, telegramUserID).Scan(&attemptCount))
	require.Equal(t, 1, attemptCount)

	var profileUsername, profileFirstName, profileLanguageCode string
	var profileIsPremium bool
	require.NoError(t, pool.QueryRow(ctx, `
SELECT username, first_name, is_premium, language_code
FROM telegram_candidates
WHERE telegram_user_id = $1`, telegramUserID).Scan(
		&profileUsername, &profileFirstName, &profileIsPremium, &profileLanguageCode,
	))
	require.Equal(t, "candidate", profileUsername)
	require.Equal(t, "Candidate", profileFirstName)
	require.True(t, profileIsPremium)
	require.Equal(t, "ru", profileLanguageCode)

	answerText := testEnvelope(t, AnswerReceived, correlationID, telegramChatID, AnswerPayload{
		CandidateIdentity: identity,
		TelegramMessageID: 101, AnswerType: "text", Text: "My answer",
	})
	messageIDs = append(messageIDs, answerText.MessageID)
	_, err = processor.Process(ctx, answerText, messaging.Shard(telegramChatID, 8))
	require.NoError(t, err)

	answerVoice := testEnvelope(t, AnswerReceived, correlationID, telegramChatID, AnswerPayload{
		CandidateIdentity: identity,
		TelegramMessageID: 102, AnswerType: "voice", VoiceFileID: "voice-file-id",
		VoiceMimeType: "audio/ogg", VoiceFileSize: 12345, VoiceDuration: 17,
	})
	messageIDs = append(messageIDs, answerVoice.MessageID)
	_, err = processor.Process(ctx, answerVoice, messaging.Shard(telegramChatID, 8))
	require.NoError(t, err)

	require.NoError(t, pool.QueryRow(ctx, `
SELECT current_question_position, status
FROM interview_sessions
WHERE id = $1`, sessionID).Scan(&currentPosition, &status))
	require.Equal(t, 1, currentPosition)
	require.Equal(t, "completed", status)

	var answerCount, deliveryCount int
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT count(*) FROM interview_answers WHERE session_id = $1", sessionID,
	).Scan(&answerCount))
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT count(*) FROM telegram_deliveries WHERE session_id = $1", sessionID,
	).Scan(&deliveryCount))
	require.Equal(t, 2, answerCount)
	require.Equal(t, 4, deliveryCount)

	var answerID uuid.UUID
	var mimeType string
	var fileSize int64
	var duration int
	require.NoError(t, pool.QueryRow(ctx, `
SELECT id, telegram_voice_mime_type, telegram_voice_file_size, telegram_voice_duration_seconds
FROM interview_answers
WHERE session_id = $1 AND answer_type = 'voice'`, sessionID).Scan(
		&answerID, &mimeType, &fileSize, &duration,
	))
	require.Equal(t, "audio/ogg", mimeType)
	require.Equal(t, int64(12345), fileSize)
	require.Equal(t, 17, duration)

	var transcriptionStatus string
	require.NoError(t, pool.QueryRow(ctx, `
SELECT status FROM interview_answer_transcriptions WHERE answer_id = $1`, answerID,
	).Scan(&transcriptionStatus))
	require.Equal(t, "pending", transcriptionStatus)

	var completedBody []byte
	require.NoError(t, pool.QueryRow(ctx, `
SELECT body FROM outbox_events
WHERE body->>'type' = $1 AND body->'payload'->>'session_id' = $2
ORDER BY created_at DESC LIMIT 1`, InterviewCompleted, sessionID.String()).Scan(&completedBody))
	var completedEnvelope messaging.Envelope
	require.NoError(t, json.Unmarshal(completedBody, &completedEnvelope))
	var completedPayload InterviewCompletedPayload
	require.NoError(t, json.Unmarshal(completedEnvelope.Payload, &completedPayload))
	require.Equal(t, sessionID, completedPayload.SessionID)
	require.Equal(t, vacancyID, completedPayload.VacancyID)
	require.Len(t, completedPayload.VoiceAnswers, 1)
	require.Equal(t, answerID, completedPayload.VoiceAnswers[0].AnswerID)
	require.Equal(t, int64(12345), completedPayload.VoiceAnswers[0].FileSize)

	totalTokens := int64(42)
	cost := 0.003
	result := testEnvelope(t, InterviewVoiceTranscribed, sessionID, telegramChatID, VoiceTranscribedPayload{
		AnswerID: answerID, SessionID: sessionID, Text: "Распознанный ответ",
		Model: "openai/gpt-4o-mini-transcribe", Attempts: 2, GenerationID: "generation-id",
		Usage: TranscriptionUsage{TotalTokens: &totalTokens, Cost: &cost},
	})
	messageIDs = append(messageIDs, result.MessageID)
	permanent, err = processor.ProcessAIResult(ctx, result)
	require.NoError(t, err)
	require.False(t, permanent)
	// The same result event is an inbox no-op.
	permanent, err = processor.ProcessAIResult(ctx, result)
	require.NoError(t, err)
	require.False(t, permanent)

	var transcript, model, generationID string
	var attempts int
	require.NoError(t, pool.QueryRow(ctx, `
SELECT status, transcription_text, model, attempts, generation_id
FROM interview_answer_transcriptions WHERE answer_id = $1`, answerID).Scan(
		&transcriptionStatus, &transcript, &model, &attempts, &generationID,
	))
	require.Equal(t, "succeeded", transcriptionStatus)
	require.Equal(t, "Распознанный ответ", transcript)
	require.Equal(t, "openai/gpt-4o-mini-transcribe", model)
	require.Equal(t, 2, attempts)
	require.Equal(t, "generation-id", generationID)

	// A late failure for an already-succeeded answer must not overwrite the transcript.
	failure := testEnvelope(t, InterviewVoiceTranscriptionFailed, sessionID, telegramChatID, VoiceTranscriptionFailedPayload{
		AnswerID: answerID, SessionID: sessionID, Model: model, Attempts: 3,
		Code: "unsupported_audio", Message: "audio format is not supported",
	})
	messageIDs = append(messageIDs, failure.MessageID)
	permanent, err = processor.ProcessAIResult(ctx, failure)
	require.NoError(t, err)
	require.False(t, permanent)
	require.NoError(t, pool.QueryRow(ctx, `
SELECT status, transcription_text FROM interview_answer_transcriptions WHERE answer_id = $1`, answerID,
	).Scan(&transcriptionStatus, &transcript))
	require.Equal(t, "succeeded", transcriptionStatus)
	require.Equal(t, "Распознанный ответ", transcript)
}

func TestProcessorExpiresSessionAndDoesNotAcceptLateAnswer(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	ownerID := uuid.New()
	vacancyID := uuid.New()
	publicToken := "expiry_" + uuid.NewString()
	telegramUserID := time.Now().UnixNano()
	correlationID := uuid.New()
	identity := candidateIdentity(telegramUserID)
	messageIDs := make([]uuid.UUID, 0, 2)
	_, err = pool.Exec(ctx, `
INSERT INTO users (id, email, password_hash)
VALUES ($1, $2, 'unused')`, ownerID, uuid.NewString()+"@example.test")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO vacancies (
    id, owner_user_id, title, interview_language, welcome_message,
    interview_duration_minutes, status, public_token
) VALUES ($1, $2, 'Expiry test', 'ru', 'Welcome', 5, 'published', $3)`,
		vacancyID, ownerID, publicToken)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO questions (id, vacancy_id, text, answer_type, position)
VALUES ($1, $2, 'Question', 'text', 0)`, uuid.New(), vacancyID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_events
WHERE body->'payload'->>'chat_id' = $1`, fmt.Sprint(telegramUserID))
		_, _ = pool.Exec(ctx, "DELETE FROM telegram_deliveries WHERE telegram_chat_id = $1", telegramUserID)
		_, _ = pool.Exec(ctx, "DELETE FROM interview_attempt_claims WHERE telegram_user_id = $1", telegramUserID)
		_, _ = pool.Exec(ctx, "DELETE FROM interview_sessions WHERE telegram_user_id = $1", telegramUserID)
		_, _ = pool.Exec(ctx, "DELETE FROM telegram_candidates WHERE telegram_user_id = $1", telegramUserID)
		for _, messageID := range messageIDs {
			_, _ = pool.Exec(ctx, "DELETE FROM inbox_messages WHERE message_id = $1", messageID)
		}
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", ownerID)
	})

	clock := time.Date(2026, 8, 31, 14, 0, 0, 0, time.UTC)
	processor := NewProcessor(pool, 8)
	processor.now = func() time.Time { return clock }
	start := testEnvelope(t, StartRequested, correlationID, telegramUserID, StartPayload{
		CandidateIdentity: identity,
		PublicToken:       publicToken,
	})
	messageIDs = append(messageIDs, start.MessageID)
	_, err = processor.Process(ctx, start, messaging.Shard(telegramUserID, 8))
	require.NoError(t, err)

	clock = clock.Add(5*time.Minute + time.Second)
	expired, err := processor.ExpireNext(ctx)
	require.NoError(t, err)
	require.True(t, expired)

	lateAnswer := testEnvelope(t, AnswerReceived, correlationID, telegramUserID, AnswerPayload{
		CandidateIdentity: identity,
		TelegramMessageID: 201,
		AnswerType:        "text",
		Text:              "Late answer",
	})
	messageIDs = append(messageIDs, lateAnswer.MessageID)
	_, err = processor.Process(ctx, lateAnswer, messaging.Shard(telegramUserID, 8))
	require.NoError(t, err)

	var status string
	var answerCount int
	require.NoError(t, pool.QueryRow(ctx, `
SELECT status FROM interview_sessions
WHERE vacancy_id = $1 AND telegram_user_id = $2`, vacancyID, telegramUserID).Scan(&status))
	require.Equal(t, "expired", status)
	require.NoError(t, pool.QueryRow(ctx, `
SELECT count(*) FROM interview_answers AS answers
JOIN interview_sessions AS sessions ON sessions.id = answers.session_id
WHERE sessions.vacancy_id = $1 AND sessions.telegram_user_id = $2`,
		vacancyID, telegramUserID).Scan(&answerCount))
	require.Zero(t, answerCount)
}

func candidateIdentity(id int64) CandidateIdentity {
	username := "candidate"
	languageCode := "ru"
	return CandidateIdentity{
		TelegramUserID: id,
		TelegramChatID: id,
		Username:       &username,
		FirstName:      "Candidate",
		IsPremium:      true,
		LanguageCode:   &languageCode,
	}
}

func testEnvelope(t *testing.T, messageType string, correlationID uuid.UUID, chatID int64, payload any) messaging.Envelope {
	t.Helper()
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)
	return messaging.Envelope{
		MessageID: uuid.New(), Type: messageType, SchemaVersion: 1,
		OccurredAt: time.Now().UTC(), CorrelationID: correlationID,
		PartitionKey: chatID, Payload: encoded,
	}
}
