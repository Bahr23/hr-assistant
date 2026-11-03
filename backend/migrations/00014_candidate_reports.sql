-- +goose Up
ALTER TABLE vacancies ADD COLUMN description text CHECK (char_length(description) <= 10000);
CREATE TABLE vacancy_evaluation_criteria (
 id uuid PRIMARY KEY, vacancy_id uuid NOT NULL REFERENCES vacancies(id) ON DELETE CASCADE,
 title text NOT NULL CHECK (char_length(btrim(title)) BETWEEN 1 AND 120),
 evaluation_guide text NOT NULL CHECK (char_length(btrim(evaluation_guide)) BETWEEN 1 AND 2000),
 position integer NOT NULL CHECK (position BETWEEN 0 AND 19), source_template_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CONSTRAINT vacancy_criteria_position_unique UNIQUE(vacancy_id, position) DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE interview_sessions ADD COLUMN vacancy_title text,
 ADD COLUMN vacancy_description text,
 ADD COLUMN evaluation_criteria jsonb NOT NULL DEFAULT '[]'::jsonb;
CREATE TABLE candidate_reports (
 session_id uuid NOT NULL REFERENCES interview_sessions(id) ON DELETE CASCADE,
 version integer NOT NULL CHECK(version > 0),
 status text NOT NULL CHECK(status IN ('pending','succeeded','permanent_failed')),
 input_snapshot jsonb, result jsonb, overall_score numeric(3,1) CHECK(overall_score BETWEEN 0 AND 10),
 scored_criteria integer NOT NULL DEFAULT 0, total_criteria integer NOT NULL DEFAULT 0,
 prompt_version text NOT NULL, schema_version text NOT NULL, model text,
 error_code text, attempts integer NOT NULL DEFAULT 0, usage jsonb,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(session_id,version),
 CHECK(status <> 'succeeded' OR (input_snapshot IS NOT NULL AND result IS NOT NULL)),
 CHECK(scored_criteria BETWEEN 0 AND total_criteria)
);
-- +goose Down
DROP TABLE candidate_reports;
ALTER TABLE interview_sessions DROP COLUMN evaluation_criteria, DROP COLUMN vacancy_description, DROP COLUMN vacancy_title;
DROP TABLE vacancy_evaluation_criteria;
ALTER TABLE vacancies DROP COLUMN description;
