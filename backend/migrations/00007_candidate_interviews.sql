-- +goose Up
CREATE TABLE interview_sessions (
    id UUID PRIMARY KEY,
    vacancy_id UUID NOT NULL REFERENCES vacancies(id),
    telegram_user_id BIGINT NOT NULL,
    telegram_chat_id BIGINT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('active', 'completed', 'cancelled')),
    current_question_position INTEGER NOT NULL DEFAULT 0 CHECK (current_question_position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX interview_sessions_one_active_per_user_idx
    ON interview_sessions(telegram_user_id)
    WHERE status = 'active';
CREATE INDEX interview_sessions_vacancy_created_idx
    ON interview_sessions(vacancy_id, created_at DESC);

CREATE TABLE interview_session_questions (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES interview_sessions(id) ON DELETE CASCADE,
    source_question_id UUID,
    text TEXT NOT NULL,
    answer_type TEXT NOT NULL CHECK (answer_type IN ('text', 'voice')),
    position INTEGER NOT NULL CHECK (position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (session_id, position)
);

CREATE TABLE interview_answers (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES interview_sessions(id) ON DELETE CASCADE,
    question_position INTEGER NOT NULL CHECK (question_position >= 0),
    telegram_message_id BIGINT NOT NULL,
    answer_type TEXT NOT NULL CHECK (answer_type IN ('text', 'voice')),
    text_answer TEXT,
    telegram_voice_file_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (session_id, question_position),
    UNIQUE (session_id, telegram_message_id),
    CHECK (
        (answer_type = 'text' AND text_answer IS NOT NULL AND telegram_voice_file_id IS NULL)
        OR
        (answer_type = 'voice' AND text_answer IS NULL AND telegram_voice_file_id IS NOT NULL)
    )
);

CREATE TABLE telegram_deliveries (
    id UUID PRIMARY KEY,
    session_id UUID REFERENCES interview_sessions(id) ON DELETE SET NULL,
    question_position INTEGER,
    telegram_chat_id BIGINT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('question', 'system', 'session_conflict')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    telegram_message_id BIGINT,
    failure_code TEXT,
    failure_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at TIMESTAMPTZ
);

CREATE INDEX telegram_deliveries_session_created_idx
    ON telegram_deliveries(session_id, created_at);

CREATE TABLE inbox_messages (
    message_id UUID PRIMARY KEY,
    message_type TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY,
    exchange_name TEXT NOT NULL,
    routing_key TEXT NOT NULL,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX outbox_events_unpublished_idx
    ON outbox_events(created_at)
    WHERE published_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS inbox_messages;
DROP TABLE IF EXISTS telegram_deliveries;
DROP TABLE IF EXISTS interview_answers;
DROP TABLE IF EXISTS interview_session_questions;
DROP TABLE IF EXISTS interview_sessions;
