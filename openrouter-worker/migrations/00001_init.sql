-- +goose Up
CREATE TABLE openrouter_worker.inbox_events (
    message_id UUID PRIMARY KEY,
    message_type TEXT NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE openrouter_worker.action_jobs (
    id UUID PRIMARY KEY,
    source_message_id UUID NOT NULL,
    action_type TEXT NOT NULL,
    subject_id UUID NOT NULL,
    session_id UUID NOT NULL,
    partition_key BIGINT NOT NULL,
    status TEXT NOT NULL CHECK (status IN (
        'pending', 'processing', 'retry_wait', 'succeeded', 'permanent_failed'
    )),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_expires_at TIMESTAMPTZ,
    last_error_code TEXT,
    last_error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    UNIQUE (action_type, subject_id)
);

CREATE INDEX action_jobs_ready_idx
    ON openrouter_worker.action_jobs(next_attempt_at, created_at)
    WHERE status IN ('pending', 'retry_wait');
CREATE INDEX action_jobs_expired_lease_idx
    ON openrouter_worker.action_jobs(lease_expires_at)
    WHERE status = 'processing';

CREATE TABLE openrouter_worker.voice_transcription_jobs (
    action_id UUID PRIMARY KEY REFERENCES openrouter_worker.action_jobs(id) ON DELETE CASCADE,
    telegram_file_id TEXT NOT NULL,
    mime_type TEXT,
    file_size BIGINT,
    duration_seconds INTEGER,
    language TEXT,
    model TEXT NOT NULL
);

CREATE TABLE openrouter_worker.outbox_events (
    id UUID PRIMARY KEY,
    exchange_name TEXT NOT NULL,
    routing_key TEXT NOT NULL,
    body JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX outbox_events_unpublished_idx
    ON openrouter_worker.outbox_events(created_at)
    WHERE published_at IS NULL;
CREATE INDEX inbox_events_received_at_idx ON openrouter_worker.inbox_events(received_at);
CREATE INDEX outbox_events_published_at_idx
    ON openrouter_worker.outbox_events(published_at) WHERE published_at IS NOT NULL;
CREATE INDEX action_jobs_terminal_updated_idx
    ON openrouter_worker.action_jobs(updated_at)
    WHERE status IN ('succeeded', 'permanent_failed');

-- +goose Down
DROP TABLE IF EXISTS openrouter_worker.outbox_events;
DROP TABLE IF EXISTS openrouter_worker.voice_transcription_jobs;
DROP TABLE IF EXISTS openrouter_worker.action_jobs;
DROP TABLE IF EXISTS openrouter_worker.inbox_events;
