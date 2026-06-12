-- HotKey Server base schema
-- Applies: users, keyword_monitors

BEGIN;

CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    display_name  TEXT        NOT NULL,
    status        TEXT        NOT NULL DEFAULT 'active',
    plan_type     TEXT        NOT NULL DEFAULT 'free',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS keyword_monitors (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT  NOT NULL REFERENCES users(id),
    name                   TEXT    NOT NULL,
    query_text             TEXT    NOT NULL,
    language               TEXT    NOT NULL DEFAULT 'en',
    region                 TEXT    NOT NULL DEFAULT 'global',
    status                 TEXT    NOT NULL DEFAULT 'active',
    poll_interval_minutes  INTEGER NOT NULL,
    alert_enabled          BOOLEAN NOT NULL DEFAULT true,
    alert_threshold_config JSONB   NOT NULL DEFAULT '{}'::jsonb,
    last_polled_at         TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_keyword_monitors_user_id ON keyword_monitors(user_id);
CREATE INDEX IF NOT EXISTS idx_keyword_monitors_status  ON keyword_monitors(status);

COMMIT;
