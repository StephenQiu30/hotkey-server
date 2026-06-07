CREATE TABLE IF NOT EXISTS cleanup_tasks (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    status text NOT NULL,
    steps jsonb NOT NULL DEFAULT '[]',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_cleanup_tasks_user_id
    ON cleanup_tasks (user_id);
