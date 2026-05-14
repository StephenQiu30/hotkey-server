-- AI Hotspot Radar initial PostgreSQL schema.
-- This file is the schema source of truth for P0.

CREATE TABLE IF NOT EXISTS keywords (
    id BIGSERIAL PRIMARY KEY,
    keyword TEXT NOT NULL UNIQUE,
    query_template TEXT,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS sources (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    source_type TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS hotspots (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    url TEXT NOT NULL,
    source_id BIGINT NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
    keyword_id BIGINT REFERENCES keywords(id) ON DELETE SET NULL,
    author TEXT,
    snippet TEXT,
    published_at TIMESTAMPTZ,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status TEXT NOT NULL DEFAULT 'new',
    raw_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_hotspots_source_url UNIQUE (source_id, url)
);

CREATE TABLE IF NOT EXISTS ai_analyses (
    id BIGSERIAL PRIMARY KEY,
    hotspot_id BIGINT NOT NULL UNIQUE REFERENCES hotspots(id) ON DELETE CASCADE,
    is_real BOOLEAN,
    relevance_score NUMERIC(5, 2) NOT NULL DEFAULT 0,
    relevance_reason TEXT,
    keyword_mentioned BOOLEAN NOT NULL DEFAULT FALSE,
    importance TEXT NOT NULL DEFAULT 'medium',
    summary TEXT,
    model_name TEXT,
    raw_response JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_ai_analyses_relevance_score CHECK (relevance_score >= 0 AND relevance_score <= 100)
);

CREATE TABLE IF NOT EXISTS reports (
    id BIGSERIAL PRIMARY KEY,
    report_type TEXT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'generated',
    subject TEXT NOT NULL,
    summary TEXT,
    content TEXT NOT NULL,
    hotspot_count INTEGER NOT NULL DEFAULT 0,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_reports_type_period UNIQUE (report_type, period_start, period_end)
);

CREATE TABLE IF NOT EXISTS notifications (
    id BIGSERIAL PRIMARY KEY,
    hotspot_id BIGINT REFERENCES hotspots(id) ON DELETE SET NULL,
    report_id BIGINT REFERENCES reports(id) ON DELETE SET NULL,
    channel TEXT NOT NULL DEFAULT 'email',
    recipient TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS check_runs (
    id BIGSERIAL PRIMARY KEY,
    trigger_type TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    status TEXT NOT NULL DEFAULT 'running',
    success_count INTEGER NOT NULL DEFAULT 0,
    failure_count INTEGER NOT NULL DEFAULT 0,
    error_summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value JSONB NOT NULL DEFAULT '{}'::jsonb,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    github_id BIGINT NOT NULL UNIQUE,
    github_login TEXT NOT NULL,
    github_name TEXT,
    email TEXT,
    avatar_url TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS ix_keywords_enabled ON keywords(enabled);
CREATE INDEX IF NOT EXISTS ix_keywords_priority ON keywords(priority);
CREATE INDEX IF NOT EXISTS ix_sources_enabled ON sources(enabled);
CREATE INDEX IF NOT EXISTS ix_sources_source_type ON sources(source_type);
CREATE INDEX IF NOT EXISTS ix_hotspots_keyword_id ON hotspots(keyword_id);
CREATE INDEX IF NOT EXISTS ix_hotspots_source_id ON hotspots(source_id);
CREATE INDEX IF NOT EXISTS ix_hotspots_published_at ON hotspots(published_at);
CREATE INDEX IF NOT EXISTS ix_hotspots_fetched_at ON hotspots(fetched_at);
CREATE INDEX IF NOT EXISTS ix_hotspots_status ON hotspots(status);
CREATE INDEX IF NOT EXISTS ix_ai_analyses_relevance_score ON ai_analyses(relevance_score);
CREATE INDEX IF NOT EXISTS ix_ai_analyses_importance ON ai_analyses(importance);
CREATE INDEX IF NOT EXISTS ix_reports_type_period ON reports(report_type, period_start, period_end);
CREATE INDEX IF NOT EXISTS ix_reports_status ON reports(status);
CREATE INDEX IF NOT EXISTS ix_notifications_status ON notifications(status);
CREATE INDEX IF NOT EXISTS ix_notifications_hotspot_id ON notifications(hotspot_id);
CREATE INDEX IF NOT EXISTS ix_notifications_report_id ON notifications(report_id);
CREATE INDEX IF NOT EXISTS ix_check_runs_status ON check_runs(status);
CREATE INDEX IF NOT EXISTS ix_check_runs_started_at ON check_runs(started_at);
CREATE INDEX IF NOT EXISTS ix_users_github_id ON users(github_id);
CREATE INDEX IF NOT EXISTS ix_users_is_active ON users(is_active);
