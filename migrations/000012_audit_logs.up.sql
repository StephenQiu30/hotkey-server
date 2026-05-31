CREATE TABLE IF NOT EXISTS audit_logs (
    id text PRIMARY KEY,
    actor_id text NOT NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    result text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (action IN ('create', 'update', 'delete')),
    CHECK (result IN ('success', 'failure'))
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_created_at
    ON audit_logs (actor_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_logs_resource_created_at
    ON audit_logs (resource_type, resource_id, created_at DESC);
