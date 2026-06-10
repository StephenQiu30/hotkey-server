-- HotKey Server PostgreSQL schema (single source of truth).
-- Apply with: psql -v ON_ERROR_STOP=1 -d <database> -f db/schema.sql

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS vector;

-- ---------------------------------------------------------------------------
-- Users & auth
-- ---------------------------------------------------------------------------

CREATE TABLE users (
    id text PRIMARY KEY,
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    role text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    timezone text NOT NULL DEFAULT 'Asia/Shanghai',
    daily_send_at text NOT NULL DEFAULT '08:30',
    email_enabled boolean NOT NULL DEFAULT true,
    weekly_enabled boolean NOT NULL DEFAULT false,
    weekly_send_at text NOT NULL DEFAULT '09:00',
    wechat_open_id text UNIQUE,
    wechat_union_id text,
    password_reset_token_hash text,
    password_reset_expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_users_role ON users (role);
CREATE INDEX idx_users_status ON users (status);
CREATE INDEX idx_users_wechat_open_id ON users (wechat_open_id);

CREATE TABLE refresh_tokens (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens (user_id);
CREATE INDEX idx_refresh_tokens_expires_at ON refresh_tokens (expires_at);

CREATE TABLE authorizations (
    id varchar(64) PRIMARY KEY,
    user_id varchar(64) NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    platform varchar(32) NOT NULL CHECK (platform IN ('github', 'wechat', 'rss', 'custom')),
    platform_user_id varchar(255) NOT NULL DEFAULT '',
    display_name varchar(255) NOT NULL DEFAULT '',
    access_token_enc text NOT NULL,
    refresh_token_enc text NOT NULL DEFAULT '',
    status varchar(32) NOT NULL DEFAULT 'connected' CHECK (status IN ('connected', 'expired', 'revoked')),
    connected_at timestamptz NOT NULL DEFAULT now(),
    last_checked_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, platform)
);

CREATE INDEX idx_authorizations_user_id ON authorizations (user_id);
CREATE INDEX idx_authorizations_platform ON authorizations (platform);
CREATE INDEX idx_authorizations_status ON authorizations (status);

-- ---------------------------------------------------------------------------
-- Channels & subscriptions
-- ---------------------------------------------------------------------------

CREATE TABLE channels (
    id text PRIMARY KEY,
    name text NOT NULL,
    slug text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_channels_status ON channels (status);

INSERT INTO channels (id, name, slug, description, status)
VALUES
    ('chn_ai_models', 'AI 模型', 'ai-models', 'AI 模型发布、能力更新与评测', 'active'),
    ('chn_ai_products', 'AI 产品', 'ai-products', 'AI 产品发布、增长与使用场景', 'active'),
    ('chn_ai_open_source', 'AI 开源', 'ai-open-source', 'AI 开源项目、框架与社区动态', 'active'),
    ('chn_ai_funding', 'AI 投融资', 'ai-funding', 'AI 公司融资、并购与资本动态', 'active')
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    updated_at = now();

CREATE TABLE user_channel_subscriptions (
    user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    channel_id text NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, channel_id)
);

CREATE INDEX idx_user_channel_subscriptions_channel_id
    ON user_channel_subscriptions (channel_id);

CREATE TABLE user_keywords (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    keyword text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_user_keywords_user_id ON user_keywords (user_id);
CREATE INDEX idx_user_keywords_enabled ON user_keywords (enabled);

CREATE TABLE system_settings (
    key text PRIMARY KEY,
    value text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO system_settings (key, value)
VALUES ('default_daily_send_at', '08:30')
ON CONFLICT (key) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Sources & collection
-- ---------------------------------------------------------------------------

CREATE TABLE sources (
    id text PRIMARY KEY,
    name text NOT NULL,
    type text NOT NULL CHECK (type IN (
        'rss', 'public_page', 'hackernews', 'wechat_mp', 'x', 'xiaohongshu'
    )),
    url text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled', 'disabled')),
    compliance_note text NOT NULL DEFAULT '',
    fetch_interval_min integer NOT NULL CHECK (fetch_interval_min > 0),
    rate_limit_per_hour integer NOT NULL DEFAULT 0 CHECK (rate_limit_per_hour >= 0),
    last_error text NOT NULL DEFAULT '',
    last_collected_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT sources_compliance_check CHECK (
        type NOT IN ('public_page', 'wechat_mp', 'x', 'xiaohongshu')
        OR compliance_note ~ E'\\S'
    )
);

CREATE INDEX idx_sources_status ON sources (status);
CREATE INDEX idx_sources_type ON sources (type);

CREATE TABLE source_channel_links (
    source_id text NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    channel_id text NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source_id, channel_id)
);

CREATE INDEX idx_source_channel_links_channel_id
    ON source_channel_links (channel_id);

CREATE TABLE collection_runs (
    id text PRIMARY KEY,
    source_id text NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('success', 'failed')),
    error_type text NOT NULL DEFAULT '',
    items_fetched integer NOT NULL DEFAULT 0 CHECK (items_fetched >= 0),
    error text NOT NULL DEFAULT '',
    started_at timestamptz NOT NULL,
    finished_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    CHECK (finished_at >= started_at),
    CHECK (status <> 'failed' OR error ~ E'\\S')
);

CREATE INDEX idx_collection_runs_source_id_started_at
    ON collection_runs (source_id, started_at DESC);

COMMENT ON COLUMN collection_runs.error_type IS
    'Classifies the failure reason: auth_failed, rate_limited, generic, or empty for success.';

CREATE TABLE x_oauth_states (
    state text PRIMARY KEY,
    code_verifier text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '10 minutes')
);

CREATE INDEX idx_x_oauth_states_expires_at ON x_oauth_states (expires_at);

CREATE TABLE x_credentials (
    source_id text PRIMARY KEY REFERENCES sources (id) ON DELETE CASCADE,
    access_token text NOT NULL,
    refresh_token text NOT NULL DEFAULT '',
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Source items & deduplication
-- ---------------------------------------------------------------------------

CREATE TABLE source_items (
    id text PRIMARY KEY,
    source_id text NOT NULL REFERENCES sources (id) ON DELETE CASCADE,
    title text NOT NULL CHECK (title ~ E'\\S'),
    snippet text NOT NULL CHECK (snippet ~ E'\\S'),
    raw_url text NOT NULL,
    canonical_url text NOT NULL,
    published_at timestamptz,
    content_hash text NOT NULL,
    language text NOT NULL DEFAULT 'unknown',
    status text NOT NULL DEFAULT 'primary' CHECK (status IN ('primary', 'duplicate')),
    duplicate_of_item_id text REFERENCES source_items (id) ON DELETE SET NULL,
    filter_status text NOT NULL DEFAULT 'unknown' CHECK (filter_status IN ('unknown', 'passed', 'filtered')),
    filter_reason text NOT NULL DEFAULT '',
    quality_score double precision NOT NULL DEFAULT 0.0,
    summarizable boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (canonical_url),
    CHECK (
        (status = 'primary' AND duplicate_of_item_id IS NULL) OR
        (status = 'duplicate' AND duplicate_of_item_id IS NOT NULL)
    )
);

CREATE INDEX idx_source_items_source_id_created_at
    ON source_items (source_id, created_at DESC);

CREATE INDEX idx_source_items_content_hash
    ON source_items (content_hash);

CREATE INDEX idx_source_items_status
    ON source_items (status);

-- ---------------------------------------------------------------------------
-- Embeddings & hotspots
-- ---------------------------------------------------------------------------

CREATE TABLE item_embeddings (
    item_id text NOT NULL REFERENCES source_items (id) ON DELETE CASCADE,
    model text NOT NULL DEFAULT 'text-embedding-v2',
    embedding vector(1536),
    text_hash text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'succeeded' CHECK (status IN ('succeeded', 'failed', 'failed_config')),
    last_error text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (item_id),
    CHECK (
        (status = 'succeeded' AND embedding IS NOT NULL) OR
        (status IN ('failed', 'failed_config'))
    )
);

CREATE INDEX idx_item_embeddings_status_updated_at
    ON item_embeddings (status, updated_at DESC);

CREATE INDEX idx_item_embeddings_embedding
    ON item_embeddings USING hnsw (embedding vector_cosine_ops)
    WHERE status = 'succeeded';

CREATE TABLE hotspot_clusters (
    id text PRIMARY KEY,
    title text NOT NULL CHECK (title ~ E'\\S'),
    keywords text[] NOT NULL DEFAULT '{}',
    centroid vector(1536) NOT NULL,
    window_start timestamptz NOT NULL,
    window_end timestamptz NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK (window_end > window_start)
);

CREATE INDEX idx_hotspot_clusters_window
    ON hotspot_clusters (window_start, window_end);

CREATE INDEX idx_hotspot_clusters_centroid
    ON hotspot_clusters USING hnsw (centroid vector_cosine_ops);

CREATE TABLE hotspot_items (
    cluster_id text NOT NULL REFERENCES hotspot_clusters (id) ON DELETE CASCADE,
    item_id text NOT NULL REFERENCES source_items (id) ON DELETE CASCADE,
    similarity double precision NOT NULL CHECK (similarity >= -1 AND similarity <= 1),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (cluster_id, item_id)
);

CREATE INDEX idx_hotspot_items_item_id
    ON hotspot_items (item_id);

CREATE TABLE hotspot_scores (
    id text PRIMARY KEY DEFAULT gen_random_uuid()::text,
    cluster_id text NOT NULL REFERENCES hotspot_clusters (id) ON DELETE CASCADE,
    total_score double precision NOT NULL DEFAULT 0,
    source_count_score double precision NOT NULL DEFAULT 0,
    freshness_score double precision NOT NULL DEFAULT 0,
    relevance_score double precision NOT NULL DEFAULT 0,
    propagation_score double precision NOT NULL DEFAULT 0,
    quality_score double precision NOT NULL DEFAULT 0,
    explanation jsonb NOT NULL DEFAULT '{}',
    score_version text NOT NULL DEFAULT 'v1',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (cluster_id, score_version)
);

CREATE INDEX idx_hotspot_scores_cluster_id
    ON hotspot_scores (cluster_id);

CREATE INDEX idx_hotspot_scores_total_score
    ON hotspot_scores (total_score DESC);

-- ---------------------------------------------------------------------------
-- AI summaries & reports
-- ---------------------------------------------------------------------------

CREATE TABLE ai_summaries (
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

CREATE UNIQUE INDEX idx_ai_summaries_cluster_prompt
    ON ai_summaries (cluster_id, prompt_version);

CREATE TABLE daily_reports (
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

CREATE UNIQUE INDEX idx_daily_reports_date_channel_user
    ON daily_reports (date, channel_id, user_id);

CREATE INDEX idx_daily_reports_status
    ON daily_reports (status);

-- ---------------------------------------------------------------------------
-- RSS, email & audit
-- ---------------------------------------------------------------------------

CREATE TABLE rss_feeds (
    user_id text NOT NULL PRIMARY KEY,
    token_hash text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    last_accessed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (token_hash)
);

CREATE TABLE email_deliveries (
    id text PRIMARY KEY,
    recipient_user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    recipient_email text NOT NULL,
    report_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('pending', 'sent', 'failed', 'failed_config')),
    attempt integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    last_error text,
    sent_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_email_deliveries_recipient_user_id
    ON email_deliveries (recipient_user_id);

CREATE INDEX idx_email_deliveries_report_id
    ON email_deliveries (report_id);

CREATE INDEX idx_email_deliveries_status
    ON email_deliveries (status);

CREATE TABLE audit_logs (
    id text PRIMARY KEY,
    actor_id text NOT NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id text,
    result text NOT NULL,
    metadata jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (action IN ('create', 'update', 'delete')),
    CHECK (result IN ('success', 'failure'))
);

CREATE INDEX idx_audit_logs_actor_created_at
    ON audit_logs (actor_id, created_at DESC);

CREATE INDEX idx_audit_logs_resource_created_at
    ON audit_logs (resource_type, resource_id, created_at DESC);

CREATE INDEX idx_audit_logs_created_at
    ON audit_logs (created_at DESC, id DESC);

-- ---------------------------------------------------------------------------
-- Jobs & admin
-- ---------------------------------------------------------------------------

CREATE TABLE jobs (
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

CREATE INDEX idx_jobs_status_scheduled_at
    ON jobs (status, scheduled_at);

CREATE INDEX idx_jobs_job_type_created_at
    ON jobs (job_type, created_at);

CREATE TABLE cleanup_tasks (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    status text NOT NULL,
    steps jsonb NOT NULL DEFAULT '[]',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_cleanup_tasks_user_id
    ON cleanup_tasks (user_id);
