-- name: ListQuestions :many
SELECT q.*
FROM questions q
JOIN vacancies v ON v.id = q.vacancy_id
WHERE q.vacancy_id = $1 AND v.owner_user_id = $2
ORDER BY q.position, q.id;

-- name: CreateQuestionAtPosition :one
INSERT INTO questions (id, vacancy_id, text, answer_type, position, evaluation_guide)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateQuestionText :one
UPDATE questions q
SET text = $4, answer_type = $5, evaluation_guide = $6, updated_at = now()
FROM vacancies v
WHERE q.id = $1
  AND q.vacancy_id = $2
  AND v.id = q.vacancy_id
  AND v.owner_user_id = $3
RETURNING q.*;

-- name: GetQuestionForOwner :one
SELECT q.*
FROM questions q
JOIN vacancies v ON v.id = q.vacancy_id
WHERE q.id = $1 AND q.vacancy_id = $2 AND v.owner_user_id = $3;

-- name: DeleteQuestionByID :execrows
DELETE FROM questions q
USING vacancies v
WHERE q.id = $1
  AND q.vacancy_id = $2
  AND v.id = q.vacancy_id
  AND v.owner_user_id = $3;

-- name: ShiftQuestionPositionsAfter :exec
UPDATE questions
SET position = position - 1, updated_at = now()
WHERE vacancy_id = $1 AND position > $2;

-- name: SetQuestionPosition :exec
UPDATE questions SET position = $3, updated_at = now()
WHERE id = $1 AND vacancy_id = $2;

-- name: LockVacancyForOwner :one
SELECT id FROM vacancies WHERE id = $1 AND owner_user_id = $2 FOR UPDATE;

-- name: NextQuestionPosition :one
SELECT COALESCE(MAX(position), -1)::integer + 1 AS position
FROM questions WHERE vacancy_id = $1;
