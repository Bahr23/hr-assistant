-- name: CreateVacancy :one
INSERT INTO vacancies (
    id, owner_user_id, title, interview_language, welcome_message,
    interview_duration_minutes, expires_at, status, public_token, description
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetVacancyByOwner :one
SELECT * FROM vacancies WHERE id = $1 AND owner_user_id = $2;

-- name: ListVacanciesByOwner :many
SELECT *
FROM vacancies
WHERE owner_user_id = $1
  AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: UpdateVacancy :one
UPDATE vacancies
SET title = $3,
    interview_language = $4,
    welcome_message = $5,
    interview_duration_minutes = $6,
    expires_at = $7,
    status = $8,
    public_token = $9,
    description = $10,
    updated_at = now()
WHERE id = $1 AND owner_user_id = $2
RETURNING *;

-- name: CountVacancyQuestions :one
SELECT count(*) FROM questions WHERE vacancy_id = $1;

-- name: ListVacancyCriteria :many
SELECT c.* FROM vacancy_evaluation_criteria c JOIN vacancies v ON v.id=c.vacancy_id
WHERE v.id=ANY($1::uuid[]) AND v.owner_user_id=$2 ORDER BY c.vacancy_id,c.position;
