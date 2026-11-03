package worker

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	candidate "hr-assistant/backend/internal/app/candidate/repository"
	feedbackstore "hr-assistant/backend/internal/app/candidatefeedback"
	"hr-assistant/backend/internal/core/messaging"
	feedback "hr-assistant/shared/candidatefeedback"
	report "hr-assistant/shared/candidatereport"
	"hr-assistant/shared/evaluation"
	"os"
	"sync"
	"testing"
	"time"
)

func TestCandidateReportLifecycle(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)
	defer pool.Close()
	owner, vacancy, criterion := uuid.New(), uuid.New(), uuid.New()
	chat := time.Now().UnixNano()
	token := uuid.NewString()
	exec := func(sql string, args ...any) { _, err := pool.Exec(ctx, sql, args...); require.NoError(t, err) }
	exec(`INSERT INTO users(id,email,password_hash)VALUES($1,$2,'unused')`, owner, uuid.NewString()+"@example.test")
	exec(`INSERT INTO vacancies(id,owner_user_id,title,description,welcome_message,interview_language,status,public_token)VALUES($1,$2,'Исходная вакансия','Исходное описание','Добро пожаловать','ru','published',$3)`, vacancy, owner, token)
	exec(`INSERT INTO vacancy_evaluation_criteria(id,vacancy_id,title,evaluation_guide,position)VALUES($1,$2,'Результат','Пример результата',0)`, criterion, vacancy)
	defer func() {
		pool.Exec(ctx, `DELETE FROM outbox_events WHERE (body->>'partition_key')::bigint=$1`, chat)
		pool.Exec(ctx, `DELETE FROM telegram_deliveries WHERE telegram_chat_id=$1`, chat)
		pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, owner)
		pool.Exec(ctx, `DELETE FROM telegram_candidates WHERE telegram_user_id=$1`, chat)
	}()
	for i, kind := range []string{"text", "text", "voice", "text"} {
		var guide *string
		if i < 2 {
			g := "Пример"
			guide = &g
		}
		exec(`INSERT INTO questions(id,vacancy_id,text,answer_type,position,evaluation_guide)VALUES($1,$2,'Исходный вопрос',$3,$4,$5)`, uuid.New(), vacancy, kind, i, guide)
	}
	p := NewProcessor(pool, 8)
	identity := candidateIdentity(chat)
	process := func(kind string, payload any) {
		e := testEnvelope(t, kind, uuid.New(), chat, payload)
		defer pool.Exec(ctx, `DELETE FROM inbox_messages WHERE message_id=$1`, e.MessageID)
		permanent, err := p.Process(ctx, e, messaging.Shard(chat, 8))
		require.NoError(t, err)
		require.False(t, permanent)
	}
	process(StartRequested, StartPayload{CandidateIdentity: identity, PublicToken: token})
	var session uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `SELECT id FROM interview_sessions WHERE telegram_user_id=$1`, chat).Scan(&session))
	exec(`UPDATE vacancies SET title='Новое название',description='Новое описание' WHERE id=$1`, vacancy)
	exec(`UPDATE vacancy_evaluation_criteria SET title='Новый критерий',evaluation_guide='Новые ориентиры' WHERE vacancy_id=$1`, vacancy)
	exec(`UPDATE questions SET text='Новый вопрос',evaluation_guide='Новые ориентиры' WHERE vacancy_id=$1`, vacancy)
	for i, kind := range []string{"text", "text", "voice", "text"} {
		a := AnswerPayload{CandidateIdentity: identity, TelegramMessageID: int64(i + 1), AnswerType: kind, Text: "Продажи выросли на 20%."}
		if kind == "voice" {
			a.Text = ""
			a.VoiceFileID = "missing"
		}
		process(AnswerReceived, a)
	}
	var completionText string
	require.NoError(t, pool.QueryRow(ctx, `SELECT body->'payload'->>'text' FROM outbox_events WHERE body->>'type'=$1 AND body->'payload'->>'session_id'=$2`, TelegramSystem, session.String()).Scan(&completionText))
	require.Equal(t, "Спасибо! Интервью завершено, ваши ответы сохранены. В ближайшее время вам придет фидбек, основанный на ваших ответах.", completionText)
	countFeedbackRequests := func() int {
		var count int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE body->>'type'=$1 AND body->'payload'->>'session_id'=$2`, feedback.Requested, session.String()).Scan(&count))
		return count
	}
	countSystemMessages := func() int {
		var count int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE body->>'type'=$1 AND body->'payload'->>'session_id'=$2`, TelegramSystem, session.String()).Scan(&count))
		return count
	}
	require.Zero(t, countFeedbackRequests())
	require.Equal(t, 1, countSystemMessages())
	store := candidate.NewStore(pool)
	view, err := store.Get(ctx, owner, session)
	require.NoError(t, err)
	require.Equal(t, "Исходная вакансия", view.Vacancy.Title)
	require.Equal(t, "pending", view.Report.Status)
	countRequests := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE body->>'type'=$1 AND body->'payload'->>'session_id'=$2`, report.Requested, session.String()).Scan(&n))
		return n
	}
	require.Zero(t, countRequests())
	processAI := func(kind string, payload any) {
		e := testEnvelope(t, kind, session, chat, payload)
		defer pool.Exec(ctx, `DELETE FROM inbox_messages WHERE message_id=$1`, e.MessageID)
		permanent, err := p.ProcessAIResult(ctx, e)
		require.NoError(t, err)
		require.False(t, permanent)
		permanent, err = p.ProcessAIResult(ctx, e)
		require.NoError(t, err)
		require.False(t, permanent)
	}
	processAI(InterviewVoiceTranscriptionFailed, VoiceTranscriptionFailedPayload{AnswerID: view.Answers[2].ID, SessionID: session, Model: "test", Attempts: 1, Code: "bad_audio", Message: "bad audio"})
	require.Zero(t, countRequests())
	completions := []evaluation.Completion{}
	for i := 0; i < 2; i++ {
		c := evaluation.Completion{AnswerID: view.Answers[i].ID, SessionID: session, Version: 1, PromptVersion: evaluation.PromptVersion, SchemaVersion: evaluation.SchemaVersion, Model: "test", Attempts: 1}
		if i == 0 {
			c.Result = json.RawMessage(`{"summary":"Есть результат.","checks":[],"follow_up_questions":[]}`)
		} else {
			c.Code = "invalid_result"
		}
		completions = append(completions, c)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, c := range completions {
		kind := evaluation.Succeeded
		if i == 1 {
			kind = evaluation.Failed
		}
		e := testEnvelope(t, kind, session, chat, c)
		defer pool.Exec(ctx, `DELETE FROM inbox_messages WHERE message_id=$1`, e.MessageID)
		wg.Add(1)
		go func(e messaging.Envelope) { defer wg.Done(); _, err := p.ProcessAIResult(ctx, e); errs <- err }(e)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 1, countRequests())
	var raw []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT input_snapshot FROM candidate_reports WHERE session_id=$1 AND version=1`, session).Scan(&raw))
	var input report.Input
	require.NoError(t, json.Unmarshal(raw, &input))
	require.Equal(t, "Исходная вакансия", input.VacancyTitle)
	require.Equal(t, "Исходное описание", *input.VacancyDescription)
	require.Equal(t, "Результат", input.Criteria[0].Title)
	require.Equal(t, "Исходный вопрос", input.Answers[0].Question)
	require.Equal(t, "Пример", *input.Answers[0].Guide)
	require.NotNil(t, input.Answers[0].Evaluation)
	require.Equal(t, "partial", input.Completeness().Status)
	require.Equal(t, 2, input.Completeness().ProcessedAnswers)
	require.Len(t, input.Criteria, 1)
	require.Equal(t, criterion, input.Criteria[0].ID)
	score := 7.26
	result := report.Result{Summary: "Есть пример результата.", CriterionScores: []report.CriterionScore{{CriterionID: criterion, Name: "Результат", Status: "scored", Score: &score, Summary: "Описан рост продаж.", Evidence: []report.Evidence{{QuestionNumber: 1, Quote: "Продажи выросли на 20%."}}, MissingInformation: []string{}}}, Strengths: []report.Strength{}, Concerns: []report.Concern{}, MissingInformation: []string{}, FollowUpQuestions: []report.FollowUp{}, AnalysisCompleteness: report.Completeness{Status: "complete", ProcessedAnswers: 4, TotalAnswers: 4, Issues: []string{}}}
	resultJSON, err := json.Marshal(result)
	require.NoError(t, err)
	completion := report.Completion{SessionID: session, Version: 1, PromptVersion: report.PromptVersion, SchemaVersion: report.SchemaVersion, Model: "test", Attempts: 1, Result: resultJSON, Usage: json.RawMessage(`{"total_tokens":123}`)}
	processAI(report.Succeeded, completion)
	require.Equal(t, 1, countRequests())
	view, err = store.Get(ctx, owner, session)
	require.NoError(t, err)
	require.Equal(t, "succeeded", view.Report.Status)
	require.Equal(t, 7.3, *view.Report.OverallScore)
	require.Equal(t, "partial", view.Report.Result.AnalysisCompleteness.Status)
	// Feedback starts after a validated report, carries no HR guides or scores,
	// and does not send anything until its own valid result is received.
	require.Equal(t, 1, countFeedbackRequests())
	require.Equal(t, 1, countSystemMessages())
	var feedbackInput []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT input_snapshot FROM candidate_feedback WHERE session_id=$1`, session).Scan(&feedbackInput))
	require.NotContains(t, string(feedbackInput), "evaluationGuide")
	require.NotContains(t, string(feedbackInput), "evaluation_guide")
	require.NotContains(t, string(feedbackInput), "criterion_scores")
	var feedbackRequest feedback.Input
	require.NoError(t, json.Unmarshal(feedbackInput, &feedbackRequest))
	require.Equal(t, "Исходная вакансия", feedbackRequest.VacancyTitle)
	require.True(t, feedbackRequest.Partial)
	feedbackResult := feedback.Completion{SessionID: session, Version: 1, PromptVersion: feedback.PromptVersion, SchemaVersion: feedback.SchemaVersion, Model: "test", Attempts: 1,
		Result: json.RawMessage(`{"paragraphs":["Спасибо за ответы. Вы привели измеримый результат — рост продаж на 20%.","Обратная связь ограничена доступными ответами. В следующий раз подробнее опишите свои действия."]}`)}
	wrongChat := testEnvelope(t, feedback.Succeeded, session, chat+1, feedbackResult)
	permanent, err := p.ProcessAIResult(ctx, wrongChat)
	require.True(t, permanent)
	require.Error(t, err)
	// Invalid feedback is not delivered and does not invalidate the report.
	invalidFeedback := feedbackResult
	invalidFeedback.Result = json.RawMessage(`{"paragraphs":["one","two","three","four"]}`)
	processAI(feedback.Succeeded, invalidFeedback)
	require.Equal(t, 1, countSystemMessages())
	var feedbackStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM candidate_feedback WHERE session_id=$1`, session).Scan(&feedbackStatus))
	require.Equal(t, "permanent_failed", feedbackStatus)
	_, err = store.RebuildReport(ctx, uuid.New(), session, 1)
	require.Error(t, err)
	// Concurrent rebuilds retry only the failed answer once, then wait for it.
	versions := make(chan int, 4)
	errorsOut := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := store.RebuildReport(ctx, owner, session, 1)
			versions <- v
			errorsOut <- err
		}()
	}
	wg.Wait()
	close(versions)
	close(errorsOut)
	for err := range errorsOut {
		require.NoError(t, err)
	}
	for v := range versions {
		require.Equal(t, 2, v)
	}
	require.Equal(t, 1, countRequests())
	view, err = store.Get(ctx, owner, session)
	require.NoError(t, err)
	require.Equal(t, "pending", view.Report.Status)
	require.Equal(t, 1, *view.Report.ResultVersion)
	require.NotNil(t, view.Report.Result)
	require.Equal(t, "pending", view.Answers[1].Evaluation.Status)
	require.Equal(t, 2, *view.Answers[1].Evaluation.Version)
	var evaluationRequests int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM outbox_events WHERE body->>'type'=$1 AND body->'payload'->>'session_id'=$2`, evaluation.Requested, session.String()).Scan(&evaluationRequests))
	require.Equal(t, 3, evaluationRequests) // Two originals and one failed check retry.
	c := completions[1]
	c.Version, c.Code, c.Result = 2, "", completions[0].Result
	processAI(evaluation.Succeeded, c)
	require.Equal(t, 2, countRequests())
	require.NoError(t, pool.QueryRow(ctx, `SELECT input_snapshot FROM candidate_reports WHERE session_id=$1 AND version=2`, session).Scan(&raw))
	require.NoError(t, json.Unmarshal(raw, &input))
	require.Equal(t, "succeeded", input.Answers[1].EvaluationStatus)
	require.Equal(t, 3, input.Completeness().ProcessedAnswers)
	// A fabricated quote fails the new version without replacing the previous success.
	completion.Version = 2
	result.CriterionScores[0].Evidence[0].Quote = "Выдуманный результат"
	completion.Result, _ = json.Marshal(result)
	processAI(report.Succeeded, completion)
	view, err = store.Get(ctx, owner, session)
	require.NoError(t, err)
	require.Equal(t, "permanent_failed", view.Report.Status)
	require.Equal(t, 1, *view.Report.ResultVersion)
	require.Equal(t, "invalid_result", *view.Report.ErrorCode)
	require.Equal(t, 1, countFeedbackRequests()) // Failed reports never schedule feedback.
	next, err := store.RebuildReport(ctx, owner, session, 2)
	require.NoError(t, err)
	require.Equal(t, 3, next)
	// Replaying an older manual retry does not schedule another answer version.
	_, err = store.RetryEvaluation(ctx, owner, session, view.Answers[1].ID, 1)
	require.NoError(t, err)
	processAI(evaluation.Succeeded, c)
	completion.Version = 3
	completion.Result = resultJSON
	processAI(report.Succeeded, completion)
	view, err = store.Get(ctx, owner, session)
	require.NoError(t, err)
	require.Equal(t, 3, *view.Report.ResultVersion)
	require.Equal(t, 3, view.Report.Result.AnalysisCompleteness.ProcessedAnswers)
	next, err = store.RebuildReport(ctx, owner, session, 2)
	require.NoError(t, err)
	require.Equal(t, 3, next)
	require.Equal(t, 3, countRequests())

	// A successful rebuild retries failed feedback using the new report version.
	require.Equal(t, 2, countFeedbackRequests())
	feedbackResult.Version = 3
	// A stale completion must not overwrite the current generation.
	processAI(feedback.Succeeded, invalidFeedback)
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM candidate_feedback WHERE session_id=$1`, session).Scan(&feedbackStatus))
	require.Equal(t, "pending", feedbackStatus)
	// A rollback leaves the result available for redelivery.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	message, permanent, err := feedbackstore.SaveResult(ctx, tx, feedback.Succeeded, chat, feedbackResult, time.Now())
	require.NoError(t, err)
	require.False(t, permanent)
	require.NotEmpty(t, message)
	require.NoError(t, tx.Rollback(ctx))
	processAI(feedback.Succeeded, feedbackResult)
	processAI(feedback.Succeeded, feedbackResult) // Fresh event IDs still cannot duplicate a message.
	require.Equal(t, 2, countSystemMessages())
	var sentText string
	require.NoError(t, pool.QueryRow(ctx, `SELECT body->'payload'->>'text' FROM outbox_events WHERE body->>'type'=$1 AND body->'payload'->>'session_id'=$2 AND body->'payload'->>'text'=$3`, TelegramSystem, session.String(), message).Scan(&sentText))
	require.Equal(t, message, sentText)
	// Failed events arriving after success cannot overwrite it or send a message.
	failedFeedback := feedbackResult
	failedFeedback.Result, failedFeedback.Code = nil, "openrouter_unavailable"
	processAI(feedback.Failed, failedFeedback)
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM candidate_feedback WHERE session_id=$1`, session).Scan(&feedbackStatus))
	require.Equal(t, "succeeded", feedbackStatus)
	require.Equal(t, 2, countSystemMessages())

	// Legacy sessions have no historical vacancy context or criteria; do not invent them.
	exec(`DELETE FROM candidate_reports WHERE session_id=$1`, session)
	exec(`UPDATE interview_sessions SET vacancy_title=NULL,vacancy_description=NULL,evaluation_criteria='[]' WHERE id=$1`, session)
	view, err = store.Get(ctx, owner, session)
	require.NoError(t, err)
	require.Equal(t, "not_started", view.Report.Status)
	next, err = store.RebuildReport(ctx, owner, session, 0)
	require.NoError(t, err)
	require.Equal(t, 1, next)
	require.NoError(t, pool.QueryRow(ctx, `SELECT input_snapshot FROM candidate_reports WHERE session_id=$1 AND version=1`, session).Scan(&raw))
	require.NoError(t, json.Unmarshal(raw, &input))
	require.True(t, input.LegacyContext)
	require.Empty(t, input.Criteria)
	require.Empty(t, input.VacancyTitle)
	result.CriterionScores = []report.CriterionScore{}
	completion.Version = 1
	completion.Result, _ = json.Marshal(result)
	processAI(report.Succeeded, completion)
	view, err = store.Get(ctx, owner, session)
	require.NoError(t, err)
	require.Equal(t, "succeeded", view.Report.Status)
	require.Nil(t, view.Report.OverallScore)
	require.Zero(t, view.Report.TotalCriteria)
	require.Equal(t, "partial", view.Report.Result.AnalysisCompleteness.Status)
	require.Equal(t, 2, countFeedbackRequests()) // Rebuild after sent feedback does not send again.
	require.Equal(t, 2, countSystemMessages())
}
