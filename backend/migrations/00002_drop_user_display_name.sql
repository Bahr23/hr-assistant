-- +goose Up
ALTER TABLE users DROP COLUMN IF EXISTS display_name;

-- +goose Down
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
