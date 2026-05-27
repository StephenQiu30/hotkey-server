-- HotKey AI 热点监控平台数据库完整 Schema
-- 设计原则：先完成全局业务域建模，再落地 API、Repository 与 Worker。
-- 执行方式：psql -v ON_ERROR_STOP=1 -d <database> -f db/schema.sql
-- 注意：本文件依赖 PostgreSQL，并需要提前安装 pgvector 扩展。

BEGIN;

-- ============================================================================
-- 1. 基础扩展与公共能力
-- ============================================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS btree_gin;

CREATE TABLE schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE countries (
    code text PRIMARY KEY,
    name text NOT NULL,
    region text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE languages (
    code text PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION touch_updated_at(table_name regclass)
RETURNS void AS $$
BEGIN
    EXECUTE format(
        'CREATE TRIGGER set_updated_at BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION set_updated_at()',
        table_name
    );
END;
$$ LANGUAGE plpgsql;


-- ============================================================================
-- 2. 租户、身份、RBAC 与计费
-- ============================================================================

CREATE TABLE tenants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    slug citext NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'deleted')),
    plan_code text NOT NULL DEFAULT 'free',
    billing_status text NOT NULL DEFAULT 'trialing'
        CHECK (billing_status IN ('trialing', 'active', 'past_due', 'canceled', 'unpaid')),
    default_locale text NOT NULL DEFAULT 'zh-CN',
    timezone text NOT NULL DEFAULT 'Asia/Shanghai',
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

SELECT touch_updated_at('tenants');

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email citext UNIQUE,
    phone text UNIQUE,
    password_hash text,
    display_name text NOT NULL,
    avatar_url text,
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'invited', 'locked', 'disabled', 'deleted')),
    last_login_at timestamptz,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    CHECK (email IS NOT NULL OR phone IS NOT NULL)
);

SELECT touch_updated_at('users');

CREATE TABLE user_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider text NOT NULL,
    provider_subject text NOT NULL,
    union_id text,
    open_id text,
    raw_profile jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_subject)
);

SELECT touch_updated_at('user_identities');

CREATE TABLE tenant_members (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    member_status text NOT NULL DEFAULT 'active'
        CHECK (member_status IN ('active', 'invited', 'disabled', 'removed')),
    title text,
    joined_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id)
);

SELECT touch_updated_at('tenant_members');

CREATE TABLE roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE,
    code text NOT NULL,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    is_system boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

SELECT touch_updated_at('roles');

CREATE TABLE permissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    resource text NOT NULL,
    action text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE role_permissions (
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE member_roles (
    member_id uuid NOT NULL REFERENCES tenant_members(id) ON DELETE CASCADE,
    role_id uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (member_id, role_id)
);

CREATE TABLE api_keys (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    key_prefix text NOT NULL,
    key_hash text NOT NULL UNIQUE,
    scopes text[] NOT NULL DEFAULT ARRAY[]::text[],
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'revoked', 'expired')),
    expires_at timestamptz,
    last_used_at timestamptz,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('api_keys');

CREATE TABLE plans (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE,
    name text NOT NULL,
    price_cents integer NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
    currency text NOT NULL DEFAULT 'USD',
    billing_interval text NOT NULL DEFAULT 'month'
        CHECK (billing_interval IN ('free', 'month', 'year', 'usage')),
    limits_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    features_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('plans');

CREATE TABLE subscriptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    plan_id uuid NOT NULL REFERENCES plans(id),
    status text NOT NULL
        CHECK (status IN ('trialing', 'active', 'past_due', 'canceled', 'unpaid')),
    provider text NOT NULL DEFAULT 'manual',
    provider_customer_id text,
    provider_subscription_id text,
    current_period_start timestamptz NOT NULL,
    current_period_end timestamptz NOT NULL,
    cancel_at timestamptz,
    canceled_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('subscriptions');

CREATE TABLE invoices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subscription_id uuid REFERENCES subscriptions(id) ON DELETE SET NULL,
    provider text NOT NULL DEFAULT 'manual',
    provider_invoice_id text,
    status text NOT NULL
        CHECK (status IN ('draft', 'open', 'paid', 'void', 'uncollectible')),
    amount_due_cents integer NOT NULL DEFAULT 0,
    amount_paid_cents integer NOT NULL DEFAULT 0,
    currency text NOT NULL DEFAULT 'USD',
    due_at timestamptz,
    paid_at timestamptz,
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE billing_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    subscription_id uuid REFERENCES subscriptions(id) ON DELETE SET NULL,
    event_type text NOT NULL,
    provider text NOT NULL,
    provider_event_id text,
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_event_id)
);

CREATE TABLE usage_counters (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    metric_key text NOT NULL,
    period_start timestamptz NOT NULL,
    period_end timestamptz NOT NULL,
    used_value bigint NOT NULL DEFAULT 0,
    limit_value bigint,
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, metric_key, period_start, period_end)
);

CREATE INDEX idx_tenant_members_user ON tenant_members (user_id);
CREATE INDEX idx_roles_tenant ON roles (tenant_id);
CREATE UNIQUE INDEX idx_roles_global_code ON roles (code) WHERE tenant_id IS NULL;
CREATE UNIQUE INDEX idx_roles_tenant_code ON roles (tenant_id, code) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_api_keys_tenant_status ON api_keys (tenant_id, status);
CREATE INDEX idx_subscriptions_tenant_status ON subscriptions (tenant_id, status);
CREATE INDEX idx_usage_counters_tenant_period ON usage_counters (tenant_id, period_start DESC);


-- ============================================================================
-- 3. 热点词、监控规则、来源治理与内容采集
-- ============================================================================

CREATE TABLE keywords (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    keyword text NOT NULL,
    normalized_keyword text NOT NULL,
    language text NOT NULL DEFAULT 'und',
    region text NOT NULL DEFAULT 'global',
    category text NOT NULL DEFAULT 'ai',
    weight numeric(8,4) NOT NULL DEFAULT 1.0,
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'paused', 'archived')),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (tenant_id, normalized_keyword, language, region)
);

SELECT touch_updated_at('keywords');

CREATE TABLE keyword_aliases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    keyword_id uuid NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    alias text NOT NULL,
    normalized_alias text NOT NULL,
    language text NOT NULL DEFAULT 'und',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (keyword_id, normalized_alias, language)
);

CREATE TABLE user_keyword_preferences (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    keyword_id uuid NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    preference_type text NOT NULL DEFAULT 'follow'
        CHECK (preference_type IN ('follow', 'mute', 'boost')),
    weight numeric(8,4) NOT NULL DEFAULT 1.0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, user_id, keyword_id)
);

SELECT touch_updated_at('user_keyword_preferences');

CREATE TABLE monitor_rules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    keyword_id uuid REFERENCES keywords(id) ON DELETE CASCADE,
    name text NOT NULL,
    rule_type text NOT NULL
        CHECK (rule_type IN ('keyword', 'source', 'semantic', 'composite')),
    include_terms text[] NOT NULL DEFAULT ARRAY[]::text[],
    exclude_terms text[] NOT NULL DEFAULT ARRAY[]::text[],
    source_scope jsonb NOT NULL DEFAULT '{}'::jsonb,
    min_trust_score numeric(6,4) NOT NULL DEFAULT 0,
    min_heat_score numeric(12,4) NOT NULL DEFAULT 0,
    config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    enabled boolean NOT NULL DEFAULT true,
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('monitor_rules');

CREATE TABLE sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    code text NOT NULL UNIQUE,
    source_type text NOT NULL
        CHECK (source_type IN ('fact', 'propagation', 'mixed')),
    reliability_level text NOT NULL
        CHECK (reliability_level IN ('official', 'research', 'media', 'community', 'social')),
    region text NOT NULL DEFAULT 'global',
    language text NOT NULL DEFAULT 'und',
    base_url text,
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'paused', 'blocked', 'archived')),
    trust_score numeric(6,4) NOT NULL DEFAULT 0.5,
    crawl_policy text NOT NULL DEFAULT 'authorized'
        CHECK (crawl_policy IN ('official_api', 'rss', 'authorized', 'manual', 'disabled')),
    robots_policy text NOT NULL DEFAULT 'respect',
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('sources');

CREATE TABLE source_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    account_type text NOT NULL
        CHECK (account_type IN ('rss', 'website', 'youtube_channel', 'x_account', 'github', 'arxiv', 'api', 'newsletter')),
    account_name text NOT NULL,
    account_url text NOT NULL,
    external_account_id text,
    verification_status text NOT NULL DEFAULT 'unverified'
        CHECK (verification_status IN ('verified', 'unverified', 'rejected')),
    trust_score numeric(6,4) NOT NULL DEFAULT 0.5,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'paused', 'blocked', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_id, account_url)
);

SELECT touch_updated_at('source_accounts');

CREATE TABLE source_credentials (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE,
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    credential_type text NOT NULL
        CHECK (credential_type IN ('api_key', 'oauth2', 'cookie', 'token', 'basic_auth')),
    encrypted_payload text NOT NULL,
    scopes text[] NOT NULL DEFAULT ARRAY[]::text[],
    expires_at timestamptz,
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'expired', 'revoked')),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('source_credentials');

CREATE TABLE source_rate_limits (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    window_seconds integer NOT NULL CHECK (window_seconds > 0),
    max_requests integer NOT NULL CHECK (max_requests > 0),
    burst integer NOT NULL DEFAULT 1 CHECK (burst > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('source_rate_limits');

CREATE TABLE source_compliance_policies (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    policy_type text NOT NULL
        CHECK (policy_type IN ('robots', 'terms', 'api_policy', 'copyright', 'manual_review')),
    policy_url text,
    allowed_use text NOT NULL DEFAULT 'unknown'
        CHECK (allowed_use IN ('allowed', 'restricted', 'forbidden', 'unknown')),
    notes text NOT NULL DEFAULT '',
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('source_compliance_policies');

CREATE TABLE collector_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE,
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    source_account_id uuid REFERENCES source_accounts(id) ON DELETE CASCADE,
    job_type text NOT NULL
        CHECK (job_type IN ('fact_poll', 'propagation_poll', 'backfill', 'refresh', 'manual')),
    schedule_cron text,
    interval_seconds integer CHECK (interval_seconds IS NULL OR interval_seconds > 0),
    priority integer NOT NULL DEFAULT 100,
    enabled boolean NOT NULL DEFAULT true,
    config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_run_at timestamptz,
    next_run_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('collector_jobs');

CREATE TABLE raw_contents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_id uuid NOT NULL REFERENCES sources(id),
    source_account_id uuid REFERENCES source_accounts(id),
    external_id text,
    url text NOT NULL,
    canonical_url text NOT NULL,
    title text NOT NULL,
    summary text,
    content_text text,
    content_type text NOT NULL
        CHECK (content_type IN ('article', 'post', 'video', 'paper', 'repo', 'release', 'comment', 'podcast')),
    language text NOT NULL DEFAULT 'und',
    region text NOT NULL DEFAULT 'global',
    author_name text,
    author_url text,
    published_at timestamptz,
    fetched_at timestamptz NOT NULL DEFAULT now(),
    content_hash text NOT NULL,
    dedupe_key text NOT NULL,
    trust_score numeric(6,4) NOT NULL DEFAULT 0.5,
    engagement_score numeric(12,4) NOT NULL DEFAULT 0,
    raw_payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'duplicate', 'hidden', 'deleted')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_id, external_id),
    UNIQUE (tenant_id, dedupe_key)
);

SELECT touch_updated_at('raw_contents');

CREATE TABLE content_assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id uuid NOT NULL REFERENCES raw_contents(id) ON DELETE CASCADE,
    asset_type text NOT NULL
        CHECK (asset_type IN ('thumbnail', 'video', 'image', 'audio', 'attachment', 'transcript')),
    url text NOT NULL,
    mime_type text,
    duration_seconds integer CHECK (duration_seconds IS NULL OR duration_seconds >= 0),
    width integer CHECK (width IS NULL OR width >= 0),
    height integer CHECK (height IS NULL OR height >= 0),
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE content_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id uuid NOT NULL REFERENCES raw_contents(id) ON DELETE CASCADE,
    title text NOT NULL,
    summary text,
    content_text text,
    content_hash text NOT NULL,
    captured_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE content_metrics (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id uuid NOT NULL REFERENCES raw_contents(id) ON DELETE CASCADE,
    metric_source text NOT NULL,
    views_count bigint,
    likes_count bigint,
    comments_count bigint,
    shares_count bigint,
    reposts_count bigint,
    collected_at timestamptz NOT NULL DEFAULT now(),
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE content_keyword_matches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id uuid NOT NULL REFERENCES raw_contents(id) ON DELETE CASCADE,
    keyword_id uuid NOT NULL REFERENCES keywords(id) ON DELETE CASCADE,
    match_score numeric(8,4) NOT NULL DEFAULT 1.0,
    match_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (content_id, keyword_id)
);

CREATE TABLE content_claims (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id uuid NOT NULL REFERENCES raw_contents(id) ON DELETE CASCADE,
    claim_text text NOT NULL,
    claim_type text NOT NULL DEFAULT 'fact'
        CHECK (claim_type IN ('fact', 'opinion', 'prediction', 'metric', 'quote')),
    subject text,
    predicate text,
    object_text text,
    confidence_score numeric(8,6) NOT NULL DEFAULT 0,
    extracted_by text NOT NULL DEFAULT 'system',
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE propagation_edges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    from_content_id uuid NOT NULL REFERENCES raw_contents(id) ON DELETE CASCADE,
    to_content_id uuid NOT NULL REFERENCES raw_contents(id) ON DELETE CASCADE,
    edge_type text NOT NULL
        CHECK (edge_type IN ('repost', 'quote', 'reply', 'reference', 'duplicate', 'derived_from')),
    confidence_score numeric(8,6) NOT NULL DEFAULT 0,
    reason text NOT NULL DEFAULT '',
    observed_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (from_content_id <> to_content_id),
    UNIQUE (from_content_id, to_content_id, edge_type)
);

CREATE INDEX idx_keywords_tenant_status ON keywords (tenant_id, status);
CREATE INDEX idx_user_keyword_preferences_user ON user_keyword_preferences (tenant_id, user_id);
CREATE INDEX idx_monitor_rules_tenant_enabled ON monitor_rules (tenant_id, enabled);
CREATE INDEX idx_sources_type_status ON sources (source_type, status);
CREATE INDEX idx_source_accounts_source_status ON source_accounts (source_id, status);
CREATE INDEX idx_collector_jobs_next_run ON collector_jobs (enabled, next_run_at);
CREATE INDEX idx_raw_contents_published_at ON raw_contents (tenant_id, published_at DESC);
CREATE INDEX idx_raw_contents_source_time ON raw_contents (source_id, published_at DESC);
CREATE INDEX idx_raw_contents_content_type ON raw_contents (tenant_id, content_type, published_at DESC);
CREATE INDEX idx_raw_contents_search ON raw_contents USING gin (to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(content_text, '')));
CREATE INDEX idx_content_metrics_content_time ON content_metrics (content_id, collected_at DESC);
CREATE INDEX idx_content_keyword_matches_keyword ON content_keyword_matches (keyword_id, match_score DESC);
CREATE INDEX idx_content_claims_content ON content_claims (content_id);
CREATE INDEX idx_propagation_edges_from ON propagation_edges (tenant_id, from_content_id);
CREATE INDEX idx_propagation_edges_to ON propagation_edges (tenant_id, to_content_id);


-- ============================================================================
-- 4. 向量索引、热点事件、证据链、事件图谱与日报
-- ============================================================================

CREATE TABLE embedding_models (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider text NOT NULL,
    model_name text NOT NULL,
    dimension integer NOT NULL CHECK (dimension > 0),
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'deprecated', 'disabled')),
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, model_name, dimension)
);

SELECT touch_updated_at('embedding_models');

CREATE TABLE content_embeddings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id uuid NOT NULL REFERENCES raw_contents(id) ON DELETE CASCADE,
    embedding_model_id uuid NOT NULL REFERENCES embedding_models(id),
    embedding vector(1536) NOT NULL,
    text_hash text NOT NULL,
    embedded_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (content_id, embedding_model_id, text_hash)
);

CREATE TABLE hotspot_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title text NOT NULL,
    summary text NOT NULL DEFAULT '',
    event_status text NOT NULL DEFAULT 'candidate'
        CHECK (event_status IN ('candidate', 'active', 'merged', 'archived', 'rejected')),
    event_type text NOT NULL DEFAULT 'general'
        CHECK (event_type IN ('general', 'product_release', 'funding', 'research', 'policy', 'incident', 'benchmark', 'partnership', 'acquisition')),
    primary_keyword_id uuid REFERENCES keywords(id) ON DELETE SET NULL,
    language text NOT NULL DEFAULT 'und',
    region text NOT NULL DEFAULT 'global',
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    peak_seen_at timestamptz,
    heat_score numeric(14,4) NOT NULL DEFAULT 0,
    trust_score numeric(6,4) NOT NULL DEFAULT 0.5,
    novelty_score numeric(8,4) NOT NULL DEFAULT 0,
    velocity_score numeric(8,4) NOT NULL DEFAULT 0,
    source_count integer NOT NULL DEFAULT 0,
    content_count integer NOT NULL DEFAULT 0,
    primary_source_id uuid REFERENCES sources(id) ON DELETE SET NULL,
    merged_into_event_id uuid REFERENCES hotspot_events(id) ON DELETE SET NULL,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

SELECT touch_updated_at('hotspot_events');

CREATE TABLE event_embeddings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
    embedding_model_id uuid NOT NULL REFERENCES embedding_models(id),
    embedding vector(1536) NOT NULL,
    text_hash text NOT NULL,
    embedded_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_id, embedding_model_id, text_hash)
);

CREATE TABLE event_contents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
    content_id uuid NOT NULL REFERENCES raw_contents(id) ON DELETE CASCADE,
    similarity_score numeric(8,6),
    match_method text NOT NULL
        CHECK (match_method IN ('vector', 'keyword', 'manual', 'llm', 'hybrid')),
    match_reason text NOT NULL DEFAULT '',
    evidence_rank integer NOT NULL DEFAULT 100,
    is_primary_evidence boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (event_id, content_id)
);

CREATE TABLE event_relations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    from_event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
    to_event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
    relation_type text NOT NULL
        CHECK (relation_type IN ('duplicate', 'follow_up', 'cause', 'contrast', 'same_topic', 'conflicts_with', 'parent_child')),
    confidence_score numeric(8,6) NOT NULL DEFAULT 0,
    reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (from_event_id <> to_event_id),
    UNIQUE (from_event_id, to_event_id, relation_type)
);

CREATE TABLE event_trust_assessments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
    assessment_type text NOT NULL
        CHECK (assessment_type IN ('source_reliability', 'cross_source_confirmed', 'conflict_detected', 'manual_review', 'llm_review')),
    score numeric(6,4) NOT NULL,
    reason text NOT NULL DEFAULT '',
    reviewer_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE event_claims (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
    content_claim_id uuid REFERENCES content_claims(id) ON DELETE SET NULL,
    claim_text text NOT NULL,
    stance text NOT NULL DEFAULT 'supporting'
        CHECK (stance IN ('supporting', 'contradicting', 'neutral', 'unknown')),
    confidence_score numeric(8,6) NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE event_conflicts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
    left_claim_id uuid NOT NULL REFERENCES event_claims(id) ON DELETE CASCADE,
    right_claim_id uuid NOT NULL REFERENCES event_claims(id) ON DELETE CASCADE,
    conflict_type text NOT NULL
        CHECK (conflict_type IN ('metric_mismatch', 'timeline_mismatch', 'source_disagreement', 'claim_contradiction')),
    status text NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'resolved', 'ignored')),
    resolution text NOT NULL DEFAULT '',
    confidence_score numeric(8,6) NOT NULL DEFAULT 0,
    resolved_by uuid REFERENCES users(id) ON DELETE SET NULL,
    resolved_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (left_claim_id <> right_claim_id)
);

CREATE TABLE similarity_rules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    scope text NOT NULL DEFAULT 'global'
        CHECK (scope IN ('global', 'tenant', 'keyword', 'source')),
    high_confidence_threshold numeric(8,6) NOT NULL DEFAULT 0.82,
    review_threshold numeric(8,6) NOT NULL DEFAULT 0.72,
    new_event_threshold numeric(8,6) NOT NULL DEFAULT 0.72,
    config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('similarity_rules');

CREATE TABLE daily_reports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    report_date date NOT NULL,
    timezone text NOT NULL DEFAULT 'Asia/Shanghai',
    title text NOT NULL,
    summary text NOT NULL DEFAULT '',
    content_markdown text NOT NULL DEFAULT '',
    content_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    generation_status text NOT NULL DEFAULT 'pending'
        CHECK (generation_status IN ('pending', 'generating', 'completed', 'failed')),
    generated_by text NOT NULL DEFAULT 'system',
    generated_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, report_date, timezone)
);

SELECT touch_updated_at('daily_reports');

CREATE TABLE report_generation_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    report_date date NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    status text NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'succeeded', 'failed', 'canceled')),
    events_considered integer NOT NULL DEFAULT 0,
    events_selected integer NOT NULL DEFAULT 0,
    error_message text,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE report_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id uuid NOT NULL REFERENCES daily_reports(id) ON DELETE CASCADE,
    event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
    rank integer NOT NULL,
    section text NOT NULL DEFAULT 'main',
    reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (report_id, event_id)
);

CREATE TABLE hotspot_rank_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    snapshot_type text NOT NULL DEFAULT 'realtime'
        CHECK (snapshot_type IN ('realtime', 'daily', 'weekly')),
    window_start timestamptz NOT NULL,
    window_end timestamptz NOT NULL,
    generated_at timestamptz NOT NULL DEFAULT now(),
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE hotspot_rank_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    snapshot_id uuid NOT NULL REFERENCES hotspot_rank_snapshots(id) ON DELETE CASCADE,
    event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
    rank integer NOT NULL,
    heat_score numeric(14,4) NOT NULL DEFAULT 0,
    rank_reason text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (snapshot_id, event_id),
    UNIQUE (snapshot_id, rank)
);

CREATE INDEX idx_content_embeddings_vector ON content_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX idx_event_embeddings_vector ON event_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX idx_hotspot_events_heat ON hotspot_events (tenant_id, event_status, heat_score DESC);
CREATE INDEX idx_hotspot_events_seen ON hotspot_events (tenant_id, last_seen_at DESC);
CREATE INDEX idx_hotspot_events_keyword ON hotspot_events (primary_keyword_id, event_status);
CREATE INDEX idx_event_contents_event ON event_contents (event_id, evidence_rank);
CREATE INDEX idx_event_contents_content ON event_contents (content_id);
CREATE INDEX idx_event_relations_from ON event_relations (tenant_id, from_event_id);
CREATE INDEX idx_event_relations_to ON event_relations (tenant_id, to_event_id);
CREATE INDEX idx_event_claims_event ON event_claims (event_id);
CREATE INDEX idx_event_conflicts_event ON event_conflicts (tenant_id, event_id, status);
CREATE INDEX idx_daily_reports_tenant_date ON daily_reports (tenant_id, report_date DESC);
CREATE INDEX idx_report_generation_runs_tenant_date ON report_generation_runs (tenant_id, report_date DESC);
CREATE INDEX idx_report_events_report_rank ON report_events (report_id, rank);
CREATE INDEX idx_hotspot_rank_snapshots_window ON hotspot_rank_snapshots (tenant_id, snapshot_type, window_end DESC);
CREATE INDEX idx_hotspot_rank_items_snapshot_rank ON hotspot_rank_items (snapshot_id, rank);


-- ============================================================================
-- 5. 任务队列、实时推送、通知、审计与系统配置
-- ============================================================================

CREATE TABLE collector_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id uuid REFERENCES collector_jobs(id) ON DELETE SET NULL,
    source_id uuid NOT NULL REFERENCES sources(id),
    source_account_id uuid REFERENCES source_accounts(id),
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    status text NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'succeeded', 'failed', 'canceled')),
    items_found integer NOT NULL DEFAULT 0,
    items_saved integer NOT NULL DEFAULT 0,
    items_skipped integer NOT NULL DEFAULT 0,
    error_code text,
    error_message text,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE queue_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE,
    queue_name text NOT NULL,
    message_type text NOT NULL,
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'completed', 'failed', 'dead')),
    priority integer NOT NULL DEFAULT 100,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    locked_by text,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('queue_messages');

CREATE TABLE worker_locks (
    lock_key text PRIMARY KEY,
    owner_id text NOT NULL,
    expires_at timestamptz NOT NULL,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('worker_locks');

CREATE TABLE realtime_channels (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel_key text NOT NULL,
    channel_type text NOT NULL
        CHECK (channel_type IN ('hotspots', 'events', 'reports', 'system')),
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'paused', 'archived')),
    config_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, channel_key)
);

SELECT touch_updated_at('realtime_channels');

CREATE TABLE realtime_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    channel_id uuid REFERENCES realtime_channels(id) ON DELETE SET NULL,
    event_type text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid,
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    sequence_no bigint NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, channel_id, sequence_no)
);

CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id uuid REFERENCES users(id) ON DELETE CASCADE,
    channel text NOT NULL
        CHECK (channel IN ('in_app', 'email', 'webhook', 'wechat')),
    subject text NOT NULL,
    body text NOT NULL DEFAULT '',
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'sent', 'failed', 'read')),
    sent_at timestamptz,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('notifications');

CREATE TABLE webhooks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name text NOT NULL,
    target_url text NOT NULL,
    secret_hash text,
    event_types text[] NOT NULL DEFAULT ARRAY[]::text[],
    status text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'paused', 'revoked')),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

SELECT touch_updated_at('webhooks');

CREATE TABLE audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid REFERENCES tenants(id) ON DELETE SET NULL,
    actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid,
    request_id text,
    ip_address inet,
    user_agent text,
    metadata_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE system_settings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE,
    setting_key text NOT NULL,
    setting_value_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    scope text NOT NULL DEFAULT 'tenant'
        CHECK (scope IN ('global', 'tenant', 'user')),
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE outbox_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    event_type text NOT NULL,
    payload_json jsonb NOT NULL DEFAULT '{}'::jsonb,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'published', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_collector_runs_source_started ON collector_runs (source_id, started_at DESC);
CREATE INDEX idx_collector_runs_job_started ON collector_runs (job_id, started_at DESC);
CREATE INDEX idx_queue_messages_ready ON queue_messages (queue_name, status, available_at, priority);
CREATE INDEX idx_worker_locks_expires ON worker_locks (expires_at);
CREATE INDEX idx_realtime_events_channel_sequence ON realtime_events (tenant_id, channel_id, sequence_no DESC);
CREATE INDEX idx_notifications_user_status ON notifications (tenant_id, user_id, status, created_at DESC);
CREATE INDEX idx_audit_logs_tenant_time ON audit_logs (tenant_id, created_at DESC);
CREATE INDEX idx_audit_logs_resource ON audit_logs (resource_type, resource_id);
CREATE UNIQUE INDEX idx_system_settings_global ON system_settings (setting_key, scope) WHERE tenant_id IS NULL;
CREATE UNIQUE INDEX idx_system_settings_tenant ON system_settings (tenant_id, setting_key, scope) WHERE tenant_id IS NOT NULL;
CREATE INDEX idx_outbox_events_ready ON outbox_events (status, available_at, created_at);



-- ============================================================================
-- 6. 中文数据字典注释
-- ============================================================================

COMMENT ON TABLE schema_migrations IS '数据库结构版本记录表，用于记录已应用的 Schema 版本。';
COMMENT ON TABLE countries IS '国家与地区字典，用于来源、内容和事件的地域归类。';
COMMENT ON TABLE languages IS '语言字典，用于来源、内容、关键词和事件的语言归类。';
COMMENT ON TABLE tenants IS '租户表，表示一个组织、团队或独立客户空间，是多租户隔离的核心。';
COMMENT ON TABLE users IS '用户表，保存登录主体和基础个人资料。';
COMMENT ON TABLE user_identities IS '第三方身份绑定表，用于微信、OAuth、邮箱等多登录方式关联。';
COMMENT ON TABLE tenant_members IS '租户成员表，连接用户与租户，并表达成员状态。';
COMMENT ON TABLE roles IS '角色表，支持系统级角色和租户级自定义角色。';
COMMENT ON TABLE permissions IS '权限点表，使用资源和动作定义可授权能力。';
COMMENT ON TABLE role_permissions IS '角色权限关联表。';
COMMENT ON TABLE member_roles IS '成员角色关联表。';
COMMENT ON TABLE api_keys IS 'API Key 表，用于开放接口、自动化任务和服务间访问。';
COMMENT ON TABLE plans IS '套餐表，定义价格、额度和功能开关。';
COMMENT ON TABLE subscriptions IS '订阅表，记录租户当前订阅状态和计费周期。';
COMMENT ON TABLE invoices IS '账单表，记录应付、已付和外部支付平台账单数据。';
COMMENT ON TABLE billing_events IS '计费事件表，用于保存支付平台或手工计费事件。';
COMMENT ON TABLE usage_counters IS '用量计数表，用于采集次数、API 调用、向量生成等额度统计。';
COMMENT ON TABLE keywords IS '热点词表，保存租户关注的 AI 关键词及权重。';
COMMENT ON TABLE keyword_aliases IS '热点词别名表，用于同义词、缩写、中英文别名匹配。';
COMMENT ON TABLE user_keyword_preferences IS '用户热点词偏好表，用于关注、屏蔽和提升权重。';
COMMENT ON TABLE monitor_rules IS '监控规则表，在关键词、来源、语义召回和组合规则之间建立可配置策略。';
COMMENT ON TABLE sources IS '来源注册表，定义事实源、传播源和混合来源的可信度与抓取策略。';
COMMENT ON TABLE source_accounts IS '来源账号表，表示 RSS、官网、YouTube 频道、X 账号、GitHub、arXiv 等具体入口。';
COMMENT ON TABLE source_credentials IS '来源授权凭证表，保存加密后的 API Key、OAuth Token 等授权信息。';
COMMENT ON TABLE source_rate_limits IS '来源限流策略表，用于控制采集频率和突发请求。';
COMMENT ON TABLE source_compliance_policies IS '来源合规策略表，用于记录 robots、条款、版权和人工审核结论。';
COMMENT ON TABLE collector_jobs IS '采集任务定义表，描述来源轮询、补采、刷新和手动采集计划。';
COMMENT ON TABLE raw_contents IS '原始内容事实表，是文章、帖子、视频、论文、发布说明等内容的事实账本。';
COMMENT ON TABLE content_assets IS '内容资产表，保存缩略图、视频、图片、音频、附件和转录文本等资源。';
COMMENT ON TABLE content_versions IS '内容版本表，用于追踪标题、摘要和正文变化。';
COMMENT ON TABLE content_metrics IS '内容传播指标表，用于保存浏览、点赞、评论、分享等时间序列指标。';
COMMENT ON TABLE content_keyword_matches IS '内容关键词命中表，记录内容与热点词的匹配分数和原因。';
COMMENT ON TABLE content_claims IS '内容事实主张表，用于抽取内容中的事实、观点、预测、指标和引用。';
COMMENT ON TABLE propagation_edges IS '内容传播边表，用于表达转发、引用、回复、重复和派生关系。';
COMMENT ON TABLE embedding_models IS '向量模型表，记录 embedding 提供方、模型名和维度。';
COMMENT ON TABLE content_embeddings IS '内容向量表，用 pgvector 保存原始内容的语义向量。';
COMMENT ON TABLE hotspot_events IS '热点事件表，保存由内容聚类生成的 AI 热点事件。';
COMMENT ON TABLE event_embeddings IS '事件向量表，用 pgvector 保存热点事件的语义向量。';
COMMENT ON TABLE event_contents IS '事件证据链表，连接热点事件与支持该事件的原始内容。';
COMMENT ON TABLE event_relations IS '事件关系表，用于完整事件图谱中的重复、后续、因果、对比、同主题和冲突关系。';
COMMENT ON TABLE event_trust_assessments IS '事件可信度评估表，记录来源可靠性、交叉验证、冲突检测、人工审核和 LLM 审核。';
COMMENT ON TABLE event_claims IS '事件事实主张表，将内容中的主张聚合到事件级别。';
COMMENT ON TABLE event_conflicts IS '事件事实冲突表，用于记录同一事件下主张之间的指标、时间线、来源和语义冲突。';
COMMENT ON TABLE similarity_rules IS '相似度规则表，用于配置向量聚类、高置信命中、人工复核和新事件阈值。';
COMMENT ON TABLE daily_reports IS '日报表，保存每日 AI 热点汇总的 Markdown 和结构化 JSON。';
COMMENT ON TABLE report_generation_runs IS '日报生成运行记录表，用于追踪生成状态、候选事件数和错误信息。';
COMMENT ON TABLE report_events IS '日报事件关联表，记录日报中选入的事件、排序和栏目。';
COMMENT ON TABLE hotspot_rank_snapshots IS '热点排名快照表，用于保存实时、日报和周报窗口的排名批次。';
COMMENT ON TABLE hotspot_rank_items IS '热点排名明细表，保存某个排名快照中的事件名次和评分原因。';
COMMENT ON TABLE collector_runs IS '采集运行记录表，记录每次采集的开始、结束、成功、失败和保存数量。';
COMMENT ON TABLE queue_messages IS '队列消息表，作为数据库侧任务队列和复杂消息队列演进的持久化基础。';
COMMENT ON TABLE worker_locks IS 'Worker 分布式锁表，用于任务互斥、刷新锁和限流锁。';
COMMENT ON TABLE realtime_channels IS '实时频道表，用于热点、事件、日报和系统消息的订阅通道。';
COMMENT ON TABLE realtime_events IS '实时事件表，保存可推送给小程序或管理后台的事件流。';
COMMENT ON TABLE notifications IS '通知表，用于站内信、邮件、Webhook 和微信通知。';
COMMENT ON TABLE webhooks IS 'Webhook 配置表，用于将热点事件和日报推送到外部系统。';
COMMENT ON TABLE audit_logs IS '审计日志表，记录用户、API 和系统对关键资源的操作。';
COMMENT ON TABLE system_settings IS '系统配置表，支持全局、租户和用户级配置。';
COMMENT ON TABLE outbox_events IS '事务外发事件表，用于可靠发布领域事件和异步集成消息。';

COMMENT ON COLUMN tenants.slug IS '租户唯一短标识，用于 URL、API 和后台管理定位。';
COMMENT ON COLUMN tenants.status IS '租户状态：active、suspended、deleted。';
COMMENT ON COLUMN tenants.billing_status IS '计费状态：trialing、active、past_due、canceled、unpaid。';
COMMENT ON COLUMN users.status IS '用户状态：active、invited、locked、disabled、deleted。';
COMMENT ON COLUMN roles.tenant_id IS '为空表示系统级角色，不为空表示租户级角色。';
COMMENT ON COLUMN sources.source_type IS '来源类型：fact 表示事实源，propagation 表示传播源，mixed 表示混合来源。';
COMMENT ON COLUMN sources.reliability_level IS '可靠性等级：official、research、media、community、social。';
COMMENT ON COLUMN sources.crawl_policy IS '采集策略：official_api、rss、authorized、manual、disabled。';
COMMENT ON COLUMN source_credentials.encrypted_payload IS '加密后的授权凭证，禁止保存明文密钥。';
COMMENT ON COLUMN raw_contents.dedupe_key IS '租户内去重键，通常由规范化 URL、标题、来源和内容哈希计算得到。';
COMMENT ON COLUMN raw_contents.trust_score IS '内容可信度分数，由来源可靠性、交叉验证和人工/LLM 审核共同影响。';
COMMENT ON COLUMN raw_contents.engagement_score IS '传播热度分数，来自浏览、点赞、评论、分享等指标。';
COMMENT ON COLUMN content_embeddings.embedding IS '内容语义向量，当前维度为 1536。';
COMMENT ON COLUMN hotspot_events.heat_score IS '热点热度分数，用于列表排序和日报候选筛选。';
COMMENT ON COLUMN hotspot_events.novelty_score IS '新颖度分数，用于区分新事件与旧事件延续。';
COMMENT ON COLUMN hotspot_events.velocity_score IS '传播速度分数，用于衡量短时间增长趋势。';
COMMENT ON COLUMN event_embeddings.embedding IS '事件语义向量，当前维度为 1536。';
COMMENT ON COLUMN event_contents.similarity_score IS '内容与事件之间的相似度分数，越高表示越相关。';
COMMENT ON COLUMN event_relations.relation_type IS '事件关系类型：duplicate、follow_up、cause、contrast、same_topic、conflicts_with、parent_child。';
COMMENT ON COLUMN similarity_rules.high_confidence_threshold IS '高置信相似度阈值，达到后可直接归并到已有事件。';
COMMENT ON COLUMN queue_messages.available_at IS '消息可被 Worker 拉取处理的最早时间。';
COMMENT ON COLUMN worker_locks.expires_at IS '锁过期时间，避免 Worker 异常退出后永久占锁。';
COMMENT ON COLUMN realtime_events.sequence_no IS '租户与频道内递增序号，用于客户端断点续传。';
COMMENT ON COLUMN audit_logs.request_id IS '请求链路 ID，用于问题排查和审计追踪。';

INSERT INTO schema_migrations (version) VALUES ('001_complete_schema');

COMMIT;
