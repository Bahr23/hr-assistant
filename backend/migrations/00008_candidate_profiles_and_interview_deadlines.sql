-- +goose Up
ALTER TABLE vacancies
    ADD COLUMN IF NOT EXISTS interview_duration_minutes INTEGER NOT NULL DEFAULT 15
        CHECK (interview_duration_minutes IN (5, 15, 30, 60));

CREATE TABLE telegram_candidates (
    telegram_user_id BIGINT PRIMARY KEY,
    telegram_chat_id BIGINT NOT NULL,
    user_data JSONB NOT NULL,
    chat_data JSONB NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE interview_sessions
    DROP CONSTRAINT interview_sessions_status_check,
    ADD CONSTRAINT interview_sessions_status_check
        CHECK (status IN ('active', 'completed', 'cancelled', 'expired')),
    ADD COLUMN expires_at TIMESTAMPTZ,
    ADD COLUMN expired_at TIMESTAMPTZ;

UPDATE interview_sessions AS sessions
SET expires_at = sessions.created_at
    + make_interval(mins => vacancies.interview_duration_minutes)
FROM vacancies
WHERE vacancies.id = sessions.vacancy_id;

ALTER TABLE interview_sessions
    ALTER COLUMN expires_at SET NOT NULL;

CREATE TABLE interview_attempt_claims (
    vacancy_id UUID NOT NULL REFERENCES vacancies(id) ON DELETE CASCADE,
    telegram_user_id BIGINT NOT NULL,
    first_session_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (vacancy_id, telegram_user_id)
);

INSERT INTO interview_attempt_claims (
    vacancy_id, telegram_user_id, first_session_id, created_at
)
SELECT DISTINCT ON (vacancy_id, telegram_user_id)
    vacancy_id, telegram_user_id, id, created_at
FROM interview_sessions
ORDER BY vacancy_id, telegram_user_id, created_at, id;

CREATE INDEX interview_sessions_active_expiry_idx
    ON interview_sessions(expires_at)
    WHERE status = 'active';

-- +goose Down
DROP INDEX IF EXISTS interview_sessions_active_expiry_idx;
DROP TABLE IF EXISTS interview_attempt_claims;

ALTER TABLE interview_sessions
    DROP COLUMN IF EXISTS expired_at,
    DROP COLUMN IF EXISTS expires_at,
    DROP CONSTRAINT interview_sessions_status_check,
    ADD CONSTRAINT interview_sessions_status_check
        CHECK (status IN ('active', 'completed', 'cancelled'));

DROP TABLE IF EXISTS telegram_candidates;

ALTER TABLE vacancies
    DROP COLUMN IF EXISTS interview_duration_minutes;
