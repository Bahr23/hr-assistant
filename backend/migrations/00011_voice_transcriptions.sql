-- +goose Up
ALTER TABLE interview_answers
    ADD COLUMN telegram_voice_mime_type TEXT,
    ADD COLUMN telegram_voice_file_size BIGINT,
    ADD COLUMN telegram_voice_duration_seconds INTEGER;

CREATE TABLE interview_answer_transcriptions (
    answer_id UUID PRIMARY KEY REFERENCES interview_answers(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('pending', 'succeeded', 'permanent_failed')),
    transcription_text TEXT,
    model TEXT,
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    generation_id TEXT,
    usage_seconds DOUBLE PRECISION,
    usage_total_tokens BIGINT,
    usage_input_tokens BIGINT,
    usage_output_tokens BIGINT,
    usage_cost_usd DOUBLE PRECISION,
    error_code TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    succeeded_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    CHECK (
        (status = 'pending' AND transcription_text IS NULL AND error_code IS NULL)
        OR
        (status = 'succeeded' AND transcription_text IS NOT NULL AND model IS NOT NULL AND succeeded_at IS NOT NULL)
        OR
        (status = 'permanent_failed' AND error_code IS NOT NULL AND failed_at IS NOT NULL)
    )
);

CREATE INDEX interview_answer_transcriptions_status_updated_idx
    ON interview_answer_transcriptions(status, updated_at);

-- +goose Down
DROP TABLE IF EXISTS interview_answer_transcriptions;

ALTER TABLE interview_answers
    DROP COLUMN IF EXISTS telegram_voice_duration_seconds,
    DROP COLUMN IF EXISTS telegram_voice_file_size,
    DROP COLUMN IF EXISTS telegram_voice_mime_type;
