-- Add error_type column to collection_runs for classifying failure reasons.
ALTER TABLE collection_runs
    ADD COLUMN error_type text NOT NULL DEFAULT '';

COMMENT ON COLUMN collection_runs.error_type IS 'Classifies the failure reason: auth_failed, rate_limited, generic, or empty for success.';
