package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	authrepository "hr-assistant/backend/internal/app/auth/repository"
	authservice "hr-assistant/backend/internal/app/auth/service"
	authtransport "hr-assistant/backend/internal/app/auth/transport"
	candidaterepository "hr-assistant/backend/internal/app/candidate/repository"
	candidateservice "hr-assistant/backend/internal/app/candidate/service"
	candidatetransport "hr-assistant/backend/internal/app/candidate/transport"
	"hr-assistant/backend/internal/app/interview/worker"
	vacancyrepository "hr-assistant/backend/internal/app/vacancy/repository"
	vacancyservice "hr-assistant/backend/internal/app/vacancy/service"
	vacancytransport "hr-assistant/backend/internal/app/vacancy/transport"
	"hr-assistant/backend/internal/core/messaging"
	"hr-assistant/shared/evaluation"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestEvaluationHTTPContract(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()
	auth, err := authservice.New(authrepository.NewSessionStore(pool), authservice.Config{
		Secret: "evaluation-test-secret-at-least-32-characters", AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour,
	})
	require.NoError(t, err)
	router := chi.NewRouter()
	router.Mount("/api/v1/auth", authtransport.Router(authtransport.Config{Service: auth}))
	router.Mount("/api/v1/vacancies", vacancytransport.Router(vacancytransport.Config{Auth: auth,
		Service: vacancyservice.New(vacancyrepository.NewDefinitionStore(pool), "https://t.me/test_bot")}))
	router.Mount("/api/v1/candidates", candidatetransport.Router(candidatetransport.Config{Auth: auth,
		Service: candidateservice.New(candidaterepository.NewStore(pool))}))
	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()
	base := server.URL + "/api/v1"
	email := uuid.NewString() + "@example.test"
	var login authResult
	require.Equal(t, http.StatusCreated, doJSON(t, client, http.MethodPost, base+"/auth/register", "", map[string]any{"email": email, "password": "password-for-evaluation-tests"}, &login))
	defer pool.Exec(ctx, `DELETE FROM users WHERE email=$1`, email)
	var vacancy vacancyResult
	require.Equal(t, http.StatusCreated, doJSON(t, client, http.MethodPost, base+"/vacancies", login.AccessToken, map[string]any{
		"title": "Проверка", "interview_language": "ru", "welcome_message": "Здравствуйте",
		"questions": []map[string]any{{"text": "Вопрос", "answer_type": "text", "evaluationGuide": "  Нужен пример  "}},
	}, &vacancy))
	// Partial updates must not materialize empty defaults for omitted fields.
	var changed map[string]any
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodPatch, base+"/vacancies/"+vacancy.ID, login.AccessToken, map[string]any{"description": "Описание позиции", "evaluationCriteria": []any{}}, &changed))
	require.Equal(t, "Проверка", changed["title"])
	require.Equal(t, "Здравствуйте", changed["welcome_message"])
	require.Equal(t, "Описание позиции", changed["description"])
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodPatch, base+"/vacancies/"+vacancy.ID, login.AccessToken, map[string]any{"description": nil}, &changed))
	require.Nil(t, changed["description"])
	var questionList struct {
		Items []struct {
			ID    string  `json:"id"`
			Guide *string `json:"evaluationGuide"`
		} `json:"items"`
	}
	questionURL := base + "/vacancies/" + vacancy.ID + "/questions"
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodGet, questionURL, login.AccessToken, nil, &questionList))
	require.Equal(t, "Нужен пример", *questionList.Items[0].Guide)
	var question map[string]any
	questionURL += "/" + questionList.Items[0].ID
	for _, guide := range []any{nil, " \t\n ", "Изменённые ориентиры"} {
		require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodPatch, questionURL, login.AccessToken, map[string]any{
			"text": "Вопрос", "answer_type": "text", "evaluationGuide": guide,
		}, &question))
		if guide == "Изменённые ориентиры" {
			require.Equal(t, guide, question["evaluationGuide"])
		} else {
			require.Nil(t, question["evaluationGuide"])
		}
	}
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodPatch, questionURL, login.AccessToken, map[string]any{
		"text": "Вопрос", "answer_type": "text",
	}, &question))
	require.Equal(t, "Изменённые ориентиры", question["evaluationGuide"])
	chatID := time.Now().UnixNano()
	var publicToken string
	require.NoError(t, pool.QueryRow(ctx, `SELECT public_token FROM vacancies WHERE id=$1`, vacancy.ID).Scan(&publicToken))
	processor := worker.NewProcessor(pool, 8)
	identity := worker.CandidateIdentity{TelegramUserID: chatID, TelegramChatID: chatID, FirstName: "Тест"}
	process := func(kind string, payload any) {
		envelope, err := messaging.NewEnvelope(kind, uuid.New(), chatID, payload)
		require.NoError(t, err)
		_, err = processor.Process(ctx, envelope, messaging.Shard(chatID, 8))
		require.NoError(t, err)
		_, _ = pool.Exec(ctx, `DELETE FROM inbox_messages WHERE message_id=$1`, envelope.MessageID)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_events WHERE (body->>'partition_key')::bigint=$1`, chatID)
		_, _ = pool.Exec(ctx, `DELETE FROM telegram_deliveries WHERE telegram_chat_id=$1`, chatID)
		_, _ = pool.Exec(ctx, `DELETE FROM telegram_candidates WHERE telegram_user_id=$1`, chatID)
	}()
	process(worker.StartRequested, worker.StartPayload{CandidateIdentity: identity, PublicToken: publicToken})
	process(worker.AnswerReceived, worker.AnswerPayload{CandidateIdentity: identity, TelegramMessageID: 1, AnswerType: "text", Text: "Я спросил клиента"})
	var sessionID, answerID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `SELECT sessions.id, answers.id FROM interview_sessions sessions JOIN interview_answers answers ON answers.session_id=sessions.id WHERE sessions.telegram_user_id=$1`, chatID).Scan(&sessionID, &answerID))
	var detail struct {
		Answers []struct {
			Evaluation struct {
				Status    string          `json:"status"`
				Result    json.RawMessage `json:"result"`
				Attempts  int             `json:"attempts"`
				ErrorCode *string         `json:"errorCode"`
			} `json:"evaluation"`
		} `json:"answers"`
	}
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodGet, base+"/candidates/"+sessionID.String(), login.AccessToken, nil, &detail))
	require.Equal(t, "pending", detail.Answers[0].Evaluation.Status)
	require.Equal(t, "null", string(detail.Answers[0].Evaluation.Result))
	retryURL := base + "/candidates/" + sessionID.String() + "/answers/" + answerID.String() + "/evaluation/retry"
	require.Equal(t, http.StatusUnauthorized, doJSON(t, client, http.MethodPost, retryURL, "", map[string]any{"version": 1}, nil))
	require.Equal(t, http.StatusConflict, doJSON(t, client, http.MethodPost, retryURL, login.AccessToken, map[string]any{"version": 1}, nil))
	envelope, err := messaging.NewEnvelope(evaluation.Failed, sessionID, chatID, evaluation.Completion{
		AnswerID: answerID, SessionID: sessionID, Version: 1, PromptVersion: evaluation.PromptVersion,
		SchemaVersion: evaluation.SchemaVersion, Model: "fake-model", Attempts: 3, Code: "invalid_result",
	})
	require.NoError(t, err)
	_, err = processor.ProcessAIResult(ctx, envelope)
	require.NoError(t, err)
	defer pool.Exec(ctx, `DELETE FROM inbox_messages WHERE message_id=$1`, envelope.MessageID)
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodGet, base+"/candidates/"+sessionID.String(), login.AccessToken, nil, &detail))
	require.Equal(t, 3, detail.Answers[0].Evaluation.Attempts)
	require.NotNil(t, detail.Answers[0].Evaluation.ErrorCode)
	require.Equal(t, "invalid_result", *detail.Answers[0].Evaluation.ErrorCode)
	for i := 0; i < 2; i++ {
		var accepted struct {
			Version int `json:"version"`
		}
		require.Equal(t, http.StatusAccepted, doJSON(t, client, http.MethodPost, retryURL, login.AccessToken, map[string]any{"version": 1}, &accepted))
		require.Equal(t, 2, accepted.Version)
	}
}
