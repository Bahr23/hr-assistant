package repository

import (
	"context"
	"errors"

	"hr-assistant/backend/internal/app/vacancy/service"
	"hr-assistant/backend/internal/core/database/dbgen"
	"hr-assistant/backend/internal/core/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DefinitionStore struct {
	pool    *pgxpool.Pool
	queries *dbgen.Queries
}

func NewDefinitionStore(pool *pgxpool.Pool) *DefinitionStore {
	return &DefinitionStore{pool: pool, queries: dbgen.New(pool)}
}

func (s *DefinitionStore) Create(ctx context.Context, definition domain.InterviewDefinition) (domain.InterviewDefinition, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.InterviewDefinition{}, err
	}
	defer tx.Rollback(ctx)
	queries := s.queries.WithTx(tx)
	vacancy := definition.Vacancy
	publicToken := pgtype.Text{}
	if vacancy.PublicToken != nil {
		publicToken = pgtype.Text{String: *vacancy.PublicToken, Valid: true}
	}
	_, err = queries.CreateVacancy(ctx, dbgen.CreateVacancyParams{
		ID: vacancy.ID, OwnerUserID: vacancy.OwnerUserID, Title: vacancy.Title, Description: optionalText(vacancy.Description),
		InterviewLanguage: vacancy.InterviewLanguage, WelcomeMessage: vacancy.WelcomeMessage,
		InterviewDurationMinutes: int32(vacancy.InterviewDurationMinutes),
		ExpiresAt:                optionalTimestamp(vacancy.ExpiresAt), Status: string(vacancy.Status), PublicToken: publicToken,
	})
	if err != nil {
		return domain.InterviewDefinition{}, definitionError(err)
	}
	for _, question := range definition.Questions {
		if _, err := tx.Exec(ctx, `
INSERT INTO questions (id, vacancy_id, text, answer_type, position, created_at, updated_at, evaluation_guide)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			question.ID, vacancy.ID, question.Text, string(question.AnswerType), question.Position,
			question.CreatedAt, question.UpdatedAt, question.EvaluationGuide,
		); err != nil {
			return domain.InterviewDefinition{}, definitionError(err)
		}
	}
	if err := saveCriteria(ctx, tx, definition.Vacancy); err != nil {
		return domain.InterviewDefinition{}, definitionError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.InterviewDefinition{}, definitionError(err)
	}
	return loadDefinition(ctx, s.queries, vacancy.OwnerUserID, vacancy.ID)
}

func (s *DefinitionStore) List(ctx context.Context, hrID uuid.UUID, query service.ListQuery) ([]domain.Vacancy, error) {
	status := pgtype.Text{}
	if query.Status != nil {
		status = pgtype.Text{String: string(*query.Status), Valid: true}
	}
	values, err := s.queries.ListVacanciesByOwner(ctx, dbgen.ListVacanciesByOwnerParams{
		OwnerUserID: hrID, Limit: query.Limit, Offset: query.Offset, Status: status,
	})
	if err != nil {
		return nil, definitionError(err)
	}
	result := make([]domain.Vacancy, 0, len(values))
	for _, value := range values {
		result = append(result, vacancyFromDB(value))
	}
	if err := loadCriteria(ctx, s.queries, hrID, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *DefinitionStore) Get(ctx context.Context, hrID, vacancyID uuid.UUID) (domain.InterviewDefinition, error) {
	return loadDefinition(ctx, s.queries, hrID, vacancyID)
}

func (s *DefinitionStore) GetPublicEntryByToken(
	ctx context.Context,
	publicToken string,
	telegramUserID int64,
) (service.PublicEntry, error) {
	var entry service.PublicEntry
	err := s.pool.QueryRow(ctx, `
SELECT vacancies.welcome_message, vacancies.description,
       EXISTS (
           SELECT 1
           FROM interview_attempt_claims AS claims
           WHERE claims.vacancy_id = vacancies.id
             AND claims.telegram_user_id = $2
       ) AS already_attempted
FROM vacancies
WHERE vacancies.public_token = $1
  AND vacancies.status = 'published'
  AND (vacancies.expires_at IS NULL OR vacancies.expires_at > now())`,
		publicToken, telegramUserID,
	).Scan(&entry.WelcomeMessage, &entry.Description, &entry.AlreadyAttempted)
	if err != nil {
		return service.PublicEntry{}, definitionError(err)
	}
	return entry, nil
}

func (s *DefinitionStore) Change(
	ctx context.Context,
	hrID, vacancyID uuid.UUID,
	mutate func(*domain.InterviewDefinition) error,
) (domain.InterviewDefinition, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.InterviewDefinition{}, err
	}
	defer tx.Rollback(ctx)
	queries := s.queries.WithTx(tx)
	if _, err := queries.LockVacancyForOwner(ctx, dbgen.LockVacancyForOwnerParams{ID: vacancyID, OwnerUserID: hrID}); err != nil {
		return domain.InterviewDefinition{}, definitionError(err)
	}
	definition, err := loadDefinition(ctx, queries, hrID, vacancyID)
	if err != nil {
		return domain.InterviewDefinition{}, err
	}
	if err := mutate(&definition); err != nil {
		return domain.InterviewDefinition{}, err
	}
	if _, err := tx.Exec(ctx, "SET CONSTRAINTS questions_vacancy_position_unique DEFERRED"); err != nil {
		return domain.InterviewDefinition{}, err
	}
	updatedVacancy, err := queries.UpdateVacancy(ctx, vacancyUpdateParams(definition.Vacancy))
	if err != nil {
		return domain.InterviewDefinition{}, definitionError(err)
	}
	questionIDs := make([]uuid.UUID, 0, len(definition.Questions))
	for _, question := range definition.Questions {
		questionIDs = append(questionIDs, question.ID)
	}
	if _, err := tx.Exec(ctx,
		"DELETE FROM questions WHERE vacancy_id = $1 AND NOT (id = ANY($2::uuid[]))",
		vacancyID, questionIDs,
	); err != nil {
		return domain.InterviewDefinition{}, definitionError(err)
	}
	for _, question := range definition.Questions {
		command, err := tx.Exec(ctx, `
INSERT INTO questions (id, vacancy_id, text, answer_type, position, created_at, updated_at, evaluation_guide)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE
SET text = EXCLUDED.text,
    answer_type = EXCLUDED.answer_type,
    evaluation_guide = EXCLUDED.evaluation_guide,
    position = EXCLUDED.position,
    updated_at = EXCLUDED.updated_at
WHERE questions.vacancy_id = EXCLUDED.vacancy_id`,
			question.ID, vacancyID, question.Text, string(question.AnswerType), question.Position,
			question.CreatedAt, question.UpdatedAt, question.EvaluationGuide,
		)
		if err != nil {
			return domain.InterviewDefinition{}, definitionError(err)
		}
		if command.RowsAffected() != 1 {
			return domain.InterviewDefinition{}, domain.NewFault(domain.FaultConflict, "question_identity_conflict", "question identity conflict")
		}
	}
	if err := saveCriteria(ctx, tx, definition.Vacancy); err != nil {
		return domain.InterviewDefinition{}, definitionError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.InterviewDefinition{}, definitionError(err)
	}
	updated := vacancyFromDB(updatedVacancy)
	updated.EvaluationCriteria = definition.Vacancy.EvaluationCriteria
	definition.Vacancy = updated
	return definition, nil
}

func loadDefinition(ctx context.Context, queries *dbgen.Queries, hrID, vacancyID uuid.UUID) (domain.InterviewDefinition, error) {
	vacancy, err := queries.GetVacancyByOwner(ctx, dbgen.GetVacancyByOwnerParams{ID: vacancyID, OwnerUserID: hrID})
	if err != nil {
		return domain.InterviewDefinition{}, definitionError(err)
	}
	questions, err := queries.ListQuestions(ctx, dbgen.ListQuestionsParams{VacancyID: vacancyID, OwnerUserID: hrID})
	if err != nil {
		return domain.InterviewDefinition{}, definitionError(err)
	}
	result := domain.InterviewDefinition{Vacancy: vacancyFromDB(vacancy), Questions: make([]domain.Question, 0, len(questions))}
	for _, question := range questions {
		result.Questions = append(result.Questions, questionFromDB(question))
	}
	values := []domain.Vacancy{result.Vacancy}
	if err := loadCriteria(ctx, queries, hrID, values); err != nil {
		return domain.InterviewDefinition{}, err
	}
	result.Vacancy = values[0]
	return result, nil
}

func vacancyUpdateParams(vacancy domain.Vacancy) dbgen.UpdateVacancyParams {
	publicToken := pgtype.Text{}
	if vacancy.PublicToken != nil {
		publicToken = pgtype.Text{String: *vacancy.PublicToken, Valid: true}
	}
	return dbgen.UpdateVacancyParams{
		ID: vacancy.ID, OwnerUserID: vacancy.OwnerUserID, Title: vacancy.Title, Description: optionalText(vacancy.Description),
		InterviewLanguage: vacancy.InterviewLanguage, WelcomeMessage: vacancy.WelcomeMessage,
		InterviewDurationMinutes: int32(vacancy.InterviewDurationMinutes),
		ExpiresAt:                optionalTimestamp(vacancy.ExpiresAt), Status: string(vacancy.Status), PublicToken: publicToken,
	}
}

func definitionError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NewFault(domain.FaultNotFound, "vacancy_not_found", "vacancy not found")
	}
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) && pgError.Code == "23505" {
		return domain.NewFault(domain.FaultConflict, "conflict", "resource conflict")
	}
	return err
}

func loadCriteria(ctx context.Context, q *dbgen.Queries, owner uuid.UUID, values []domain.Vacancy) error {
	ids := make([]uuid.UUID, len(values))
	byID := map[uuid.UUID]int{}
	for i := range values {
		ids[i] = values[i].ID
		byID[ids[i]] = i
		values[i].EvaluationCriteria = []domain.EvaluationCriterion{}
	}
	rows, err := q.ListVacancyCriteria(ctx, dbgen.ListVacancyCriteriaParams{Column1: ids, OwnerUserID: owner})
	if err != nil {
		return err
	}
	for _, c := range rows {
		source := c.SourceTemplateID
		i := byID[c.VacancyID]
		values[i].EvaluationCriteria = append(values[i].EvaluationCriteria, domain.EvaluationCriterion{ID: c.ID, Title: c.Title, EvaluationGuide: c.EvaluationGuide, Position: int(c.Position), SourceTemplateID: source})
	}
	return nil
}
func saveCriteria(ctx context.Context, tx pgx.Tx, v domain.Vacancy) error {
	ids := make([]uuid.UUID, 0, len(v.EvaluationCriteria))
	for _, c := range v.EvaluationCriteria {
		ids = append(ids, c.ID)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM vacancy_evaluation_criteria WHERE vacancy_id=$1 AND NOT(id=ANY($2::uuid[]))`, v.ID, ids); err != nil {
		return err
	}
	for _, c := range v.EvaluationCriteria {
		command, err := tx.Exec(ctx, `INSERT INTO vacancy_evaluation_criteria(id,vacancy_id,title,evaluation_guide,position,source_template_id) VALUES($1,$2,$3,$4,$5,$6)
  ON CONFLICT(id) DO UPDATE SET title=EXCLUDED.title,evaluation_guide=EXCLUDED.evaluation_guide,position=EXCLUDED.position,source_template_id=EXCLUDED.source_template_id,updated_at=now()
  WHERE vacancy_evaluation_criteria.vacancy_id=EXCLUDED.vacancy_id`, c.ID, v.ID, c.Title, c.EvaluationGuide, c.Position, c.SourceTemplateID)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return domain.NewFault(domain.FaultConflict, "criterion_identity_conflict", "criterion identity conflict")
		}
	}
	return nil
}
