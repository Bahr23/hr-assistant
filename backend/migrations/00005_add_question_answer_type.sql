-- +goose Up
ALTER TABLE questions
    ADD COLUMN IF NOT EXISTS answer_type TEXT NOT NULL DEFAULT 'voice'
    CHECK (answer_type IN ('text', 'voice'));

-- +goose Down
ALTER TABLE questions DROP COLUMN IF EXISTS answer_type;
