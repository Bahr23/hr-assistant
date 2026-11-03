-- +goose Up
ALTER TABLE telegram_candidates
    ADD COLUMN username TEXT,
    ADD COLUMN first_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN last_name TEXT,
    ADD COLUMN is_premium BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN language_code TEXT;

UPDATE telegram_candidates
SET username = NULLIF(user_data->>'username', ''),
    first_name = COALESCE(user_data->>'first_name', ''),
    last_name = NULLIF(user_data->>'last_name', ''),
    is_premium = COALESCE((user_data->>'is_premium')::boolean, false),
    language_code = NULLIF(user_data->>'language_code', '');

ALTER TABLE telegram_candidates
    DROP COLUMN user_data,
    DROP COLUMN chat_data;

-- +goose Down
ALTER TABLE telegram_candidates
    ADD COLUMN user_data JSONB,
    ADD COLUMN chat_data JSONB;

UPDATE telegram_candidates
SET user_data = jsonb_strip_nulls(jsonb_build_object(
        'id', telegram_user_id,
        'is_bot', false,
        'username', username,
        'first_name', first_name,
        'last_name', last_name,
        'is_premium', is_premium,
        'language_code', language_code
    )),
    chat_data = jsonb_strip_nulls(jsonb_build_object(
        'id', telegram_chat_id,
        'type', 'private',
        'username', username,
        'first_name', first_name,
        'last_name', last_name
    ));

ALTER TABLE telegram_candidates
    ALTER COLUMN user_data SET NOT NULL,
    ALTER COLUMN chat_data SET NOT NULL,
    DROP COLUMN username,
    DROP COLUMN first_name,
    DROP COLUMN last_name,
    DROP COLUMN is_premium,
    DROP COLUMN language_code;
