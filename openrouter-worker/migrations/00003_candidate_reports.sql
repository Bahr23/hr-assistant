-- +goose Up
CREATE TABLE openrouter_worker.candidate_report_jobs (
 action_id uuid PRIMARY KEY REFERENCES openrouter_worker.action_jobs(id) ON DELETE CASCADE,
 input jsonb NOT NULL, model text NOT NULL, prompt_version text NOT NULL, schema_version text NOT NULL
);
CREATE TABLE openrouter_worker.completed_candidate_reports (
 session_id uuid NOT NULL, version integer NOT NULL, PRIMARY KEY(session_id,version)
);
-- +goose Down
DROP TABLE openrouter_worker.completed_candidate_reports;
DROP TABLE openrouter_worker.candidate_report_jobs;
DELETE FROM openrouter_worker.action_jobs WHERE action_type='candidate_report';
