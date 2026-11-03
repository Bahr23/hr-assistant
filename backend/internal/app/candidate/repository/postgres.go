package repository

import (
	"context"
	"encoding/json"
	"errors"
	"hr-assistant/backend/internal/app/answerevaluation"
	"hr-assistant/backend/internal/app/candidatereport"
	"hr-assistant/shared/evaluation"
	"time"

	"hr-assistant/backend/internal/app/candidate/service"
	"hr-assistant/backend/internal/core/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const candidateFilters = `
FROM interview_sessions AS sessions
JOIN vacancies ON vacancies.id = sessions.vacancy_id
JOIN telegram_candidates AS candidates ON candidates.telegram_user_id = sessions.telegram_user_id
WHERE vacancies.owner_user_id = $1
  AND sessions.status = 'completed'
  AND ($2::uuid IS NULL OR vacancies.id = $2)
  AND ($3::text = '' OR strpos(lower(concat_ws(' ', candidates.first_name, candidates.last_name, candidates.username)), lower($3)) > 0)`

func (s *Store) List(ctx context.Context, hrID uuid.UUID, query service.ListQuery) (service.Page, error) {
	var total int64
	err := s.pool.QueryRow(ctx, "SELECT count(*) "+candidateFilters, hrID, query.VacancyID, query.Search).Scan(&total)
	if err != nil {
		return service.Page{}, err
	}
	rows, err := s.pool.Query(ctx, `
SELECT sessions.id,
       candidates.telegram_user_id, candidates.username, candidates.first_name,
       candidates.last_name, candidates.is_premium, candidates.language_code,
       candidates.first_seen_at, candidates.last_seen_at,
       vacancies.id, COALESCE(sessions.vacancy_title,vacancies.title),
       sessions.created_at,
       COALESCE(sessions.completed_at, sessions.updated_at),
       GREATEST(0, EXTRACT(EPOCH FROM (COALESCE(sessions.completed_at, sessions.updated_at) - sessions.created_at))::bigint),
       (SELECT count(*) FROM interview_answers AS answers WHERE answers.session_id = sessions.id)::integer
`+candidateFilters+`
ORDER BY COALESCE(sessions.completed_at, sessions.updated_at) DESC, sessions.id DESC
LIMIT $4 OFFSET $5`, hrID, query.VacancyID, query.Search, query.Limit, query.Offset)
	if err != nil {
		return service.Page{}, err
	}
	defer rows.Close()

	items := make([]service.Summary, 0)
	for rows.Next() {
		var value service.Summary
		err = rows.Scan(
			&value.ID,
			&value.Candidate.TelegramUserID, &value.Candidate.Username, &value.Candidate.FirstName,
			&value.Candidate.LastName, &value.Candidate.IsPremium, &value.Candidate.LanguageCode,
			&value.Candidate.FirstSeenAt, &value.Candidate.LastSeenAt,
			&value.Vacancy.ID, &value.Vacancy.Title,
			&value.StartedAt, &value.CompletedAt, &value.DurationSeconds, &value.AnswerCount,
		)
		if err != nil {
			return service.Page{}, err
		}
		items = append(items, value)
	}
	if err := rows.Err(); err != nil {
		return service.Page{}, err
	}
	return service.Page{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (s *Store) Get(ctx context.Context, hrID, sessionID uuid.UUID) (service.Interview, error) {
	var result service.Interview
	err := s.pool.QueryRow(ctx, `
SELECT sessions.id,
       candidates.telegram_user_id, candidates.username, candidates.first_name,
       candidates.last_name, candidates.is_premium, candidates.language_code,
       candidates.first_seen_at, candidates.last_seen_at,
       vacancies.id, COALESCE(sessions.vacancy_title,vacancies.title),
       sessions.created_at,
       COALESCE(sessions.completed_at, sessions.updated_at),
       GREATEST(0, EXTRACT(EPOCH FROM (COALESCE(sessions.completed_at, sessions.updated_at) - sessions.created_at))::bigint)
FROM interview_sessions AS sessions
JOIN vacancies ON vacancies.id = sessions.vacancy_id
JOIN telegram_candidates AS candidates ON candidates.telegram_user_id = sessions.telegram_user_id
WHERE sessions.id = $1
  AND vacancies.owner_user_id = $2
  AND sessions.status = 'completed'`, sessionID, hrID).Scan(
		&result.ID,
		&result.Candidate.TelegramUserID, &result.Candidate.Username, &result.Candidate.FirstName,
		&result.Candidate.LastName, &result.Candidate.IsPremium, &result.Candidate.LanguageCode,
		&result.Candidate.FirstSeenAt, &result.Candidate.LastSeenAt,
		&result.Vacancy.ID, &result.Vacancy.Title,
		&result.StartedAt, &result.CompletedAt, &result.DurationSeconds,
	)
	if err != nil {
		return service.Interview{}, mapError(err)
	}

	rows, err := s.pool.Query(ctx, `
SELECT answers.id, questions.position, questions.text, answers.answer_type,
       answers.created_at, answers.text_answer,
       transcriptions.status, transcriptions.transcription_text, questions.evaluation_guide,
       latest.status, latest.version, succeeded.result, succeeded.version,
       COALESCE(latest.attempt_count,0),latest.last_error
FROM interview_session_questions AS questions
JOIN interview_answers AS answers
  ON answers.session_id = questions.session_id
 AND answers.question_position = questions.position
LEFT JOIN interview_answer_transcriptions AS transcriptions ON transcriptions.answer_id = answers.id
LEFT JOIN LATERAL (
    SELECT status, version, attempt_count, last_error FROM interview_answer_evaluations WHERE answer_id = answers.id ORDER BY version DESC LIMIT 1
) AS latest ON true
LEFT JOIN LATERAL (
    SELECT result, version FROM interview_answer_evaluations WHERE answer_id = answers.id AND status = 'succeeded' ORDER BY version DESC LIMIT 1
) AS succeeded ON true
WHERE questions.session_id = $1
ORDER BY questions.position`, sessionID)
	if err != nil {
		return service.Interview{}, err
	}
	defer rows.Close()

	result.Answers = make([]service.Answer, 0)
	for rows.Next() {
		var answer service.Answer
		var transcriptionStatus *string
		var transcriptionText *string
		var guide, evaluationStatus *string
		var resultJSON []byte
		if err := rows.Scan(
			&answer.ID, &answer.Position, &answer.Question, &answer.AnswerType,
			&answer.AnsweredAt, &answer.Text, &transcriptionStatus, &transcriptionText, &guide,
			&evaluationStatus, &answer.Evaluation.Version, &resultJSON, &answer.Evaluation.ResultVersion,
			&answer.Evaluation.Attempts, &answer.Evaluation.ErrorCode,
		); err != nil {
			return service.Interview{}, err
		}
		if answer.AnswerType == "voice" {
			status := "pending"
			if transcriptionStatus != nil {
				status = *transcriptionStatus
			}
			answer.Transcription = &service.Transcription{Status: status, Text: transcriptionText}
		}
		answer.Evaluation.Status = "pending"
		switch {
		case evaluationStatus != nil:
			answer.Evaluation.Status = *evaluationStatus
		case guide == nil:
			answer.Evaluation.Status = "skipped"
			reason := "no_guide"
			answer.Evaluation.Reason = &reason
		case answer.Transcription != nil && answer.Transcription.Status == "permanent_failed":
			answer.Evaluation.Status = "skipped"
			reason := "transcription_failed"
			answer.Evaluation.Reason = &reason
		case answer.Transcription != nil && answer.Transcription.Status != "succeeded":
			reason := "waiting_transcription"
			answer.Evaluation.Reason = &reason
		}
		if resultJSON != nil {
			var result evaluation.Result
			if err := json.Unmarshal(resultJSON, &result); err != nil {
				return service.Interview{}, err
			}
			answer.Evaluation.Result = &result
		}
		result.Answers = append(result.Answers, answer)
	}
	if err := rows.Err(); err != nil {
		return service.Interview{}, err
	}
	result.Report, err = candidatereport.Load(ctx, s.pool, sessionID)
	if err != nil {
		return service.Interview{}, err
	}
	return result, nil
}

func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NewFault(domain.FaultNotFound, "candidate_not_found", "candidate interview not found")
	}
	return err
}

func (s *Store) RetryEvaluation(ctx context.Context, hrID, sessionID, answerID uuid.UUID, version int) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	next, err := answerevaluation.Retry(ctx, tx, hrID, sessionID, answerID, version, time.Now().UTC())
	if errors.Is(err, answerevaluation.ErrNotRetryable) {
		return 0, domain.NewFault(domain.FaultConflict, "evaluation_not_retryable", "only failed evaluations can be retried")
	}
	if err != nil {
		return 0, mapError(err)
	}
	return next, tx.Commit(ctx)
}

func (s *Store) RebuildReport(ctx context.Context, hrID, sessionID uuid.UUID, version int) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	next, err := candidatereport.Retry(ctx, tx, hrID, sessionID, version, time.Now().UTC())
	if errors.Is(err, candidatereport.ErrNotRetryable) {
		return 0, domain.NewFault(domain.FaultConflict, "report_not_retryable", "report is already pending or version is stale")
	}
	if err != nil {
		return 0, mapError(err)
	}
	return next, tx.Commit(ctx)
}
