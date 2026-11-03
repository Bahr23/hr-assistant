-- +goose Up
ALTER TABLE questions ADD COLUMN evaluation_guide TEXT
    CHECK (evaluation_guide IS NULL OR (char_length(evaluation_guide) BETWEEN 1 AND 4000 AND btrim(evaluation_guide) <> ''));
ALTER TABLE interview_session_questions ADD COLUMN evaluation_guide TEXT
    CHECK (evaluation_guide IS NULL OR (char_length(evaluation_guide) BETWEEN 1 AND 4000 AND btrim(evaluation_guide) <> ''));

CREATE TABLE interview_answer_evaluations (
    answer_id UUID NOT NULL REFERENCES interview_answers(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    status TEXT NOT NULL CHECK (status IN ('pending', 'succeeded', 'permanent_failed')),
    result JSONB,
    model TEXT,
    prompt_version TEXT NOT NULL,
    schema_version TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (answer_id, version),
    CHECK ((status = 'succeeded') = (result IS NOT NULL)),
    CHECK ((status = 'pending') = (completed_at IS NULL))
);

-- +goose Down
DROP TABLE interview_answer_evaluations;
ALTER TABLE interview_session_questions DROP COLUMN evaluation_guide;
ALTER TABLE questions DROP COLUMN evaluation_guide;
