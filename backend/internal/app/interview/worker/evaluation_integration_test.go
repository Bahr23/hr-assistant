package worker

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	candidaterepository "hr-assistant/backend/internal/app/candidate/repository"
	"hr-assistant/backend/internal/core/domain"
	"hr-assistant/backend/internal/core/messaging"
	"hr-assistant/shared/evaluation"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestAnswerEvaluationLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()
	ownerID, vacancyID := uuid.New(), uuid.New()
	chatID := time.Now().UnixNano()
	correlationID := uuid.New()
	publicToken := "evaluation_" + uuid.NewString()
	exec := func(sql string, args ...any) { _, err := pool.Exec(ctx, sql, args...); require.NoError(t, err) }
	exec(`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'unused')`, ownerID, uuid.NewString()+"@example.test")
	exec(`INSERT INTO vacancies (id, owner_user_id, title, welcome_message, interview_language, status, public_token)
VALUES ($1, $2, 'Evaluation test', 'Welcome', 'ru', 'published', $3)`, vacancyID, ownerID, publicToken)
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_events WHERE (body->>'partition_key')::bigint = $1`, chatID)
		_, _ = pool.Exec(ctx, `DELETE FROM telegram_deliveries WHERE telegram_chat_id = $1`, chatID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
		_, _ = pool.Exec(ctx, `DELETE FROM telegram_candidates WHERE telegram_user_id = $1`, chatID)
	}()
	for i, kind := range []string{"text", "voice", "voice", "text"} {
		var guide *string
		if i < 3 {
			text := "Пример действий и результат"
			guide = &text
		}
		exec(`INSERT INTO questions (id, vacancy_id, text, answer_type, position, evaluation_guide) VALUES ($1,$2,'Исходный вопрос',$3,$4,$5)`, uuid.New(), vacancyID, kind, i, guide)
	}
	processor := NewProcessor(pool, 8)
	identity := candidateIdentity(chatID)
	process := func(kind string, payload any) {
		envelope := testEnvelope(t, kind, correlationID, chatID, payload)
		defer pool.Exec(ctx, `DELETE FROM inbox_messages WHERE message_id = $1`, envelope.MessageID)
		permanent, err := processor.Process(ctx, envelope, messaging.Shard(chatID, 8))
		require.NoError(t, err)
		require.False(t, permanent)
	}
	process(StartRequested, StartPayload{CandidateIdentity: identity, PublicToken: publicToken})
	var sessionID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM interview_sessions WHERE telegram_user_id=$1`, chatID).Scan(&sessionID))
	// HR changes the live vacancy after start; even future evaluations use the snapshot.
	exec(`UPDATE questions SET text='Изменённый вопрос', evaluation_guide='Изменённые ориентиры' WHERE vacancy_id=$1`, vacancyID)
	countEvaluations := func() int {
		var count int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM interview_answer_evaluations e JOIN interview_answers a ON a.id=e.answer_id WHERE a.session_id=$1`, sessionID).Scan(&count))
		return count
	}
	for i, kind := range []string{"text", "voice", "voice", "text"} {
		answer := AnswerPayload{CandidateIdentity: identity, TelegramMessageID: int64(100 + i), AnswerType: kind}
		if kind == "text" {
			answer.Text = "Я спросил клиента"
		} else {
			answer.VoiceFileID = "voice-file"
		}
		process(AnswerReceived, answer)
		if i < 3 {
			require.Zero(t, countEvaluations())
		}
	}
	require.Equal(t, 1, countEvaluations())
	var request evaluation.Request
	var requestJSON []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT body->'payload' FROM outbox_events WHERE body->>'type'=$1 AND body->'payload'->>'session_id'=$2`, evaluation.Requested, sessionID.String()).Scan(&requestJSON))
	require.NoError(t, json.Unmarshal(requestJSON, &request))
	require.Equal(t, "Исходный вопрос", request.Input.Question)
	require.Equal(t, "Пример действий и результат", request.Input.Guide)
	require.Equal(t, "text", request.Input.AnswerSource)
	var telegramBodies string
	require.NoError(t, pool.QueryRow(ctx, `SELECT string_agg(body::text, '') FROM outbox_events WHERE exchange_name=$1 AND (body->>'partition_key')::bigint=$2`, messaging.TelegramExchange, chatID).Scan(&telegramBodies))
	require.NotContains(t, telegramBodies, "evaluation_guide")
	require.NotContains(t, telegramBodies, request.Input.Guide)
	store := candidaterepository.NewStore(pool)
	interview, err := store.Get(ctx, ownerID, sessionID)
	require.NoError(t, err)
	textID, voiceID, failedVoiceID := interview.Answers[0].ID, interview.Answers[1].ID, interview.Answers[2].ID
	require.Equal(t, "pending", interview.Answers[0].Evaluation.Status)
	require.Equal(t, "waiting_transcription", *interview.Answers[1].Evaluation.Reason)
	require.Equal(t, "skipped", interview.Answers[3].Evaluation.Status)
	processAI := func(kind string, payload any, valid bool) {
		envelope := testEnvelope(t, kind, sessionID, chatID, payload)
		defer pool.Exec(ctx, `DELETE FROM inbox_messages WHERE message_id = $1`, envelope.MessageID)
		permanent, err := processor.ProcessAIResult(ctx, envelope)
		if !valid {
			require.Error(t, err)
			require.True(t, permanent)
			return
		}
		require.NoError(t, err)
		require.False(t, permanent)
		// Same envelope must be idempotent as well as a duplicate with a different message ID.
		permanent, err = processor.ProcessAIResult(ctx, envelope)
		require.NoError(t, err)
		require.False(t, permanent)
	}
	processAI(InterviewVoiceTranscriptionFailed, VoiceTranscriptionFailedPayload{AnswerID: failedVoiceID, SessionID: sessionID, Model: "audio-model", Attempts: 1, Code: "bad_audio", Message: "invalid audio"}, true)
	require.Equal(t, 1, countEvaluations())
	transcription := VoiceTranscribedPayload{AnswerID: voiceID, SessionID: sessionID, Text: "Я спросил клиента", Model: "audio-model", Attempts: 2}
	processAI(InterviewVoiceTranscribed, transcription, true)
	processAI(InterviewVoiceTranscribed, transcription, true)
	require.Equal(t, 2, countEvaluations())
	interview, err = store.Get(ctx, ownerID, sessionID)
	require.NoError(t, err)
	require.Equal(t, "transcription_failed", *interview.Answers[2].Evaluation.Reason)
	completion := evaluation.Completion{AnswerID: textID, SessionID: sessionID, Version: 1, PromptVersion: evaluation.PromptVersion,
		SchemaVersion: evaluation.SchemaVersion, Model: "test-model", Attempts: 1,
		Result: json.RawMessage(`{"summary":"Пример есть.","checks":[{"criterion":"Пример","type":"expected","status":"found","evidence":["Выдуманная цитата"],"explanation":"Есть пример."}],"follow_up_questions":[]}`)}
	processAI(evaluation.Succeeded, completion, false)
	completion.Result = json.RawMessage(`{"summary":"Пример есть.","checks":[{"criterion":"Пример","type":"expected","status":"found","evidence":["Я спросил"],"explanation":"Есть пример."}],"follow_up_questions":[]}`)
	processAI(evaluation.Succeeded, completion, true)
	completion.Result = nil
	completion.Code = "invalid_result"
	processAI(evaluation.Failed, completion, true) // Late failure must not replace success.
	completion.AnswerID = voiceID
	processAI(evaluation.Failed, completion, true)
	_, err = store.RetryEvaluation(ctx, uuid.New(), sessionID, voiceID, 1)
	var fault *domain.Fault
	require.ErrorAs(t, err, &fault)
	require.Equal(t, domain.FaultNotFound, fault.Kind)
	_, err = store.RetryEvaluation(ctx, ownerID, sessionID, textID, 1)
	require.ErrorAs(t, err, &fault)
	require.Equal(t, domain.FaultConflict, fault.Kind)
	var wait sync.WaitGroup
	for i := 0; i < 4; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			next, err := store.RetryEvaluation(ctx, ownerID, sessionID, voiceID, 1)
			require.NoError(t, err)
			require.Equal(t, 2, next)
		}()
	}
	wait.Wait()
	require.Equal(t, 3, countEvaluations())
	completion.Version = 2
	completion.Code = ""
	completion.Result = json.RawMessage(`{"summary":"Уточнил причину.","checks":[],"follow_up_questions":[]}`)
	processAI(evaluation.Succeeded, completion, true)
	next, err := store.RetryEvaluation(ctx, ownerID, sessionID, voiceID, 1) // Lost HTTP response replay.
	require.NoError(t, err)
	require.Equal(t, 2, next)
	require.Equal(t, 3, countEvaluations())
	// Future reanalysis can retain the latest success while a new version runs/fails.
	exec(`INSERT INTO interview_answer_evaluations (answer_id, version, status, prompt_version, schema_version) VALUES ($1,3,'pending',$2,$3)`, voiceID, evaluation.PromptVersion, evaluation.SchemaVersion)
	interview, err = store.Get(ctx, ownerID, sessionID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", interview.Answers[0].Evaluation.Status)
	require.Equal(t, "pending", interview.Answers[1].Evaluation.Status)
	require.Equal(t, 2, *interview.Answers[1].Evaluation.ResultVersion)
	require.Equal(t, "Уточнил причину.", interview.Answers[1].Evaluation.Result.Summary)
	completion.Version = 3
	completion.Result = nil
	completion.Code = "invalid_result"
	processAI(evaluation.Failed, completion, true)
	interview, err = store.Get(ctx, ownerID, sessionID)
	require.NoError(t, err)
	require.Equal(t, "permanent_failed", interview.Answers[1].Evaluation.Status)
	require.Equal(t, 2, *interview.Answers[1].Evaluation.ResultVersion)
}
