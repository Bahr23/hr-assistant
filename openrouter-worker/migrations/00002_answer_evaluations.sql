-- +goose Up
ALTER TABLE openrouter_worker.action_jobs ADD COLUMN analysis_version INTEGER NOT NULL DEFAULT 1 CHECK (analysis_version > 0);
ALTER TABLE openrouter_worker.action_jobs DROP CONSTRAINT action_jobs_action_type_subject_id_key;
ALTER TABLE openrouter_worker.action_jobs ADD CONSTRAINT action_jobs_action_subject_version_key
    UNIQUE (action_type, subject_id, analysis_version);
ALTER TABLE openrouter_worker.action_jobs ADD COLUMN invalid_attempts INTEGER NOT NULL DEFAULT 0;

CREATE TABLE openrouter_worker.answer_evaluation_jobs (
    action_id UUID PRIMARY KEY REFERENCES openrouter_worker.action_jobs(id) ON DELETE CASCADE,
    input JSONB NOT NULL,
    model TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    schema_version TEXT NOT NULL
);
-- Permanent compact receipts preserve idempotency even after job/inbox retention expires.
CREATE TABLE openrouter_worker.completed_evaluations (
    answer_id UUID NOT NULL,
    version INTEGER NOT NULL,
    PRIMARY KEY (answer_id, version)
);

-- +goose Down
DROP TABLE openrouter_worker.completed_evaluations;
DROP TABLE openrouter_worker.answer_evaluation_jobs;
DELETE FROM openrouter_worker.action_jobs WHERE action_type = 'answer_evaluation';
ALTER TABLE openrouter_worker.action_jobs DROP CONSTRAINT action_jobs_action_subject_version_key;
ALTER TABLE openrouter_worker.action_jobs DROP COLUMN analysis_version;
ALTER TABLE openrouter_worker.action_jobs DROP COLUMN invalid_attempts;
ALTER TABLE openrouter_worker.action_jobs ADD UNIQUE (action_type, subject_id);
