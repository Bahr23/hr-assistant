-- +goose Up
CREATE TABLE candidate_feedback (
 session_id uuid PRIMARY KEY REFERENCES interview_sessions(id) ON DELETE CASCADE,
 report_version integer NOT NULL CHECK(report_version > 0),
 status text NOT NULL CHECK(status IN ('pending','succeeded','permanent_failed')),
 input_snapshot jsonb NOT NULL, result jsonb,
 prompt_version text NOT NULL, schema_version text NOT NULL,
 model text, attempts integer NOT NULL DEFAULT 0, error_code text,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(status <> 'succeeded' OR result IS NOT NULL)
);
-- +goose Down
DROP TABLE candidate_feedback;
