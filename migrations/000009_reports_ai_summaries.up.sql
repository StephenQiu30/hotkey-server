CREATE TABLE IF NOT EXISTS ai_summaries (
    id text PRIMARY KEY,
    cluster_id text NOT NULL,
    prompt_version text NOT NULL,
    summary text NOT NULL,
    status text NOT NULL CHECK (status IN ('succeeded', 'degraded', 'failed_config', 'failed')),
    last_error text NOT NULL DEFAULT '',
    source_refs_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_summaries_cluster_prompt
    ON ai_summaries (cluster_id, prompt_version);

CREATE TABLE IF NOT EXISTS daily_reports (
    id text PRIMARY KEY,
    date date NOT NULL,
    channel_id text NOT NULL DEFAULT '',
    user_id text NOT NULL DEFAULT '',
    prompt_version text NOT NULL,
    input_hotspot_ids_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    body text NOT NULL,
    status text NOT NULL CHECK (status IN ('succeeded', 'degraded', 'failed_config', 'failed')),
    last_error text NOT NULL DEFAULT '',
    source_refs_json jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_daily_reports_date_channel_user
    ON daily_reports (date, channel_id, user_id);

CREATE INDEX IF NOT EXISTS idx_daily_reports_status
    ON daily_reports (status);
