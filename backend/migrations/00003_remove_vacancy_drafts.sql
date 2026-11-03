-- +goose Up
DELETE FROM vacancies WHERE status = 'draft';

ALTER TABLE vacancies DROP CONSTRAINT IF EXISTS vacancies_status_check;
ALTER TABLE vacancies
    ALTER COLUMN status SET DEFAULT 'published',
    ADD CONSTRAINT vacancies_status_check CHECK (status IN ('published', 'archived'));

-- +goose Down
ALTER TABLE vacancies DROP CONSTRAINT IF EXISTS vacancies_status_check;
ALTER TABLE vacancies
    ALTER COLUMN status SET DEFAULT 'draft',
    ADD CONSTRAINT vacancies_status_check CHECK (status IN ('draft', 'published', 'archived'));
