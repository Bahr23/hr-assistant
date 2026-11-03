-- +goose Up
ALTER TABLE vacancies
    DROP COLUMN IF EXISTS company_name,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS requirements,
    DROP COLUMN IF EXISTS employment_conditions;

-- +goose Down
ALTER TABLE vacancies
    ADD COLUMN company_name TEXT NOT NULL DEFAULT '',
    ADD COLUMN description TEXT NOT NULL DEFAULT '',
    ADD COLUMN requirements TEXT NOT NULL DEFAULT '',
    ADD COLUMN employment_conditions TEXT NOT NULL DEFAULT '';
