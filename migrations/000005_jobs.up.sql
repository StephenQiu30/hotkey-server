CREATE TABLE IF NOT EXISTS jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_type text NOT NULL,
    payload jsonb NOT NULL,
    status text NOT NULL,
    attempt integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts integer NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
    idempotency_key text NOT NULL,
    last_error text,
    scheduled_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (idempotency_key),
    CHECK (attempt <= max_attempts)
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_scheduled_at
    ON jobs (status, scheduled_at);

CREATE INDEX IF NOT EXISTS idx_jobs_job_type_created_at
    ON jobs (job_type, created_at);
