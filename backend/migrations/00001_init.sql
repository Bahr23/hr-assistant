-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

CREATE TABLE vacancies (
    id UUID PRIMARY KEY,
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    interview_language TEXT NOT NULL DEFAULT '',
    welcome_message TEXT NOT NULL DEFAULT '',
    interview_duration_minutes INTEGER NOT NULL DEFAULT 15
        CHECK (interview_duration_minutes IN (5, 15, 30, 60)),
    expires_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'published' CHECK (status IN ('published', 'archived')),
    public_token TEXT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX vacancies_owner_status_created_idx
    ON vacancies(owner_user_id, status, created_at DESC);

CREATE TABLE questions (
    id UUID PRIMARY KEY,
    vacancy_id UUID NOT NULL REFERENCES vacancies(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    answer_type TEXT NOT NULL DEFAULT 'text' CHECK (answer_type IN ('text', 'voice')),
    position INTEGER NOT NULL CHECK (position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT questions_vacancy_position_unique
        UNIQUE (vacancy_id, position) DEFERRABLE INITIALLY IMMEDIATE
);

CREATE INDEX questions_vacancy_id_idx ON questions(vacancy_id);

-- +goose Down
DROP TABLE IF EXISTS questions;
DROP TABLE IF EXISTS vacancies;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
