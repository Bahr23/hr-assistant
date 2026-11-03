-- +goose Up
ALTER TABLE questions ALTER COLUMN answer_type SET DEFAULT 'text';

-- +goose Down
ALTER TABLE questions ALTER COLUMN answer_type SET DEFAULT 'voice';
