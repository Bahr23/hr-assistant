-- +goose Up
CREATE INDEX interview_sessions_completed_vacancy_idx
    ON interview_sessions(vacancy_id, completed_at DESC, id DESC)
    WHERE status = 'completed';

-- +goose Down
DROP INDEX IF EXISTS interview_sessions_completed_vacancy_idx;
