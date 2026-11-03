package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
	reports "hr-assistant/backend/internal/app/candidatereport"
	"hr-assistant/backend/internal/app/interview/worker"
	"hr-assistant/backend/internal/core/messaging"
	"hr-assistant/shared/evaluation"
)

// Run only against an isolated stack with an HTTP mock configured as OpenRouter.
func TestEvaluationAndReportThroughRabbitMQ(t *testing.T) {
	baseURL, rabbitURL, databaseURL := os.Getenv("E2E_BASE_URL"), os.Getenv("TEST_RABBITMQ_URL"), os.Getenv("TEST_DATABASE_URL")
	if os.Getenv("EVALUATION_QUEUE_E2E") != "1" || baseURL == "" || rabbitURL == "" || databaseURL == "" {
		t.Skip("isolated evaluation queue stack is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	defer pool.Close()
	connection, err := amqp.Dial(rabbitURL)
	require.NoError(t, err)
	defer connection.Close()
	channel, err := connection.Channel()
	require.NoError(t, err)
	defer channel.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	email := "evaluation-" + uuid.NewString() + "@example.test"
	var login authResult
	require.Equal(t, http.StatusCreated, doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/register", "", map[string]any{
		"email": email, "password": "evaluation-test-password",
	}, &login))
	var catalog struct {
		Items []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Guide string `json:"evaluationGuide"`
		}
	}
	require.Equal(t, http.StatusUnauthorized, doJSON(t, client, http.MethodGet, baseURL+"/api/v1/evaluation-criteria/templates", "", nil, nil))
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodGet, baseURL+"/api/v1/evaluation-criteria/templates", login.AccessToken, nil, &catalog))
	require.Len(t, catalog.Items, 10)
	criteria := []map[string]any{{"id": uuid.NewString(), "title": "Работа с клиентами", "evaluationGuide": "Уточнение потребности и итог разговора", "position": 0, "sourceTemplateId": catalog.Items[0].ID}, {"id": uuid.NewString(), "title": "Командная работа", "evaluationGuide": "Пример совместной работы", "position": 1, "sourceTemplateId": nil}}
	var vacancy vacancyResult
	require.Equal(t, http.StatusCreated, doJSON(t, client, http.MethodPost, baseURL+"/api/v1/vacancies", login.AccessToken, map[string]any{
		"title": "Менеджер по продажам — тест отчёта", "description": "Работа с клиентами и развитие продаж.", "evaluationCriteria": criteria, "welcome_message": "Здравствуйте!", "interview_language": "ru",
		"questions": []map[string]any{{"text": "Как вы работали с возражением клиента о высокой цене?", "answer_type": "text", "evaluationGuide": "Конкретный пример, уточнение причины возражения и итог разговора."}},
	}, &vacancy))
	var saved map[string]any
	vacancyURL := baseURL + "/api/v1/vacancies/" + vacancy.ID
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodGet, vacancyURL, login.AccessToken, nil, &saved))
	require.Equal(t, "Работа с клиентами и развитие продаж.", saved["description"])
	require.Len(t, saved["evaluationCriteria"], 2)
	invalid := []map[string]any{{"id": uuid.NewString(), "title": "Пустой критерий", "evaluationGuide": " ", "position": 0, "sourceTemplateId": nil}}
	require.Equal(t, http.StatusUnprocessableEntity, doJSON(t, client, http.MethodPatch, vacancyURL, login.AccessToken, map[string]any{"description": "Не сохранять", "evaluationCriteria": invalid}, nil))
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodGet, vacancyURL, login.AccessToken, nil, &saved))
	require.Equal(t, "Работа с клиентами и развитие продаж.", saved["description"])
	chatID := time.Now().UnixNano()
	identity := worker.CandidateIdentity{TelegramUserID: chatID, TelegramChatID: chatID, FirstName: "Тестовый", LastName: ptrString("Кандидат")}
	var token string
	require.NoError(t, pool.QueryRow(ctx, `SELECT public_token FROM vacancies WHERE id=$1`, vacancy.ID).Scan(&token))
	publish := func(kind string, payload any) {
		envelope, err := messaging.NewEnvelope(kind, uuid.New(), chatID, payload)
		require.NoError(t, err)
		body, err := json.Marshal(envelope)
		require.NoError(t, err)
		key := fmt.Sprintf("candidate.%d.%s", messaging.Shard(chatID, 8), strings.TrimPrefix(kind, "candidate."))
		require.NoError(t, channel.PublishWithContext(ctx, messaging.CandidateExchange, key, false, false, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: body}))
	}
	publish(worker.StartRequested, worker.StartPayload{CandidateIdentity: identity, PublicToken: token})
	var sessionID uuid.UUID
	require.Eventually(t, func() bool {
		return pool.QueryRow(ctx, `SELECT id FROM interview_sessions WHERE telegram_user_id=$1`, chatID).Scan(&sessionID) == nil
	}, 15*time.Second, 100*time.Millisecond)
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodPatch, vacancyURL, login.AccessToken, map[string]any{"description": "Изменено после старта", "evaluationCriteria": []any{}}, nil))
	publish(worker.AnswerReceived, worker.AnswerPayload{CandidateIdentity: identity, TelegramMessageID: 1, AnswerType: "text", Text: "Я спросил клиента, с чем он сравнивает цену."})
	var status string
	require.Eventually(t, func() bool {
		err := pool.QueryRow(ctx, `SELECT e.status FROM interview_answer_evaluations e JOIN interview_answers a ON a.id=e.answer_id WHERE a.session_id=$1`, sessionID).Scan(&status)
		return err == nil && status == "succeeded"
	}, 20*time.Second, 100*time.Millisecond)
	var detail struct {
		Report  reports.View `json:"report"`
		Answers []struct {
			Evaluation struct {
				Status string            `json:"status"`
				Result evaluation.Result `json:"result"`
			} `json:"evaluation"`
		} `json:"answers"`
	}
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodGet, baseURL+"/api/v1/candidates/"+sessionID.String(), login.AccessToken, nil, &detail))
	require.Equal(t, "succeeded", detail.Answers[0].Evaluation.Status)
	require.NotEmpty(t, detail.Answers[0].Evaluation.Result.Summary)
	require.Equal(t, "Я спросил клиента, с чем он сравнивает цену.", detail.Answers[0].Evaluation.Result.Checks[0].Evidence[0])
	require.Eventually(t, func() bool {
		var status string
		return pool.QueryRow(ctx, `SELECT status FROM candidate_reports WHERE session_id=$1 AND version=1`, sessionID).Scan(&status) == nil && status == "succeeded"
	}, 20*time.Second, 100*time.Millisecond)
	candidateURL := baseURL + "/api/v1/candidates/" + sessionID.String()
	require.Equal(t, http.StatusOK, doJSON(t, client, http.MethodGet, candidateURL, login.AccessToken, nil, &detail))
	require.Equal(t, "succeeded", detail.Report.Status)
	require.Equal(t, 7.3, *detail.Report.OverallScore)
	require.Equal(t, 1, detail.Report.ScoredCriteria)
	require.Equal(t, 2, detail.Report.TotalCriteria)
	require.Equal(t, "Работа с клиентами", detail.Report.Result.CriterionScores[0].Name)
	require.Equal(t, "insufficient_data", detail.Report.Result.CriterionScores[1].Status)
	require.Equal(t, "complete", detail.Report.Result.AnalysisCompleteness.Status)
	var foreign authResult
	require.Equal(t, http.StatusCreated, doJSON(t, client, http.MethodPost, baseURL+"/api/v1/auth/register", "", map[string]any{"email": uuid.NewString() + "@example.test", "password": "evaluation-test-password"}, &foreign))
	require.Equal(t, http.StatusNotFound, doJSON(t, client, http.MethodGet, candidateURL, foreign.AccessToken, nil, nil))
	require.Equal(t, http.StatusNotFound, doJSON(t, client, http.MethodPost, candidateURL+"/report/rebuild", foreign.AccessToken, map[string]int{"version": 1}, nil))
	var next struct {
		Version int `json:"version"`
	}
	for range 2 {
		require.Equal(t, http.StatusAccepted, doJSON(t, client, http.MethodPost, candidateURL+"/report/rebuild", login.AccessToken, map[string]int{"version": 1}, &next))
		require.Equal(t, 2, next.Version)
	}
	require.Eventually(t, func() bool {
		var status string
		return pool.QueryRow(ctx, `SELECT status FROM candidate_reports WHERE session_id=$1 AND version=2`, sessionID).Scan(&status) == nil && status == "succeeded"
	}, 20*time.Second, 100*time.Millisecond)
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM openrouter_worker.action_jobs WHERE session_id=$1 AND action_type='candidate_report'`, sessionID).Scan(&count))
	require.Equal(t, 2, count)
	var usage []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT usage FROM candidate_reports WHERE session_id=$1 AND version=2`, sessionID).Scan(&usage))
	require.Contains(t, string(usage), "700")
	t.Logf("Test fixture: email=%s session=%s", email, sessionID)
}

func ptrString(value string) *string { return &value }
