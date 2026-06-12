-- HotKey Server schema
-- Applies: users, keyword_monitors, Plan 003 ingestion tables

BEGIN;

-- Users (Plan 002)
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

-- Keyword monitors (Plan 002)
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

-- Monitor runs (Plan 003)
CREATE TABLE IF NOT EXISTS monitor_runs (
    id              BIGSERIAL PRIMARY KEY,
    monitor_id      BIGINT      NOT NULL REFERENCES keyword_monitors(id),
    platform        TEXT        NOT NULL,
    run_type        TEXT        NOT NULL,
    status          TEXT        NOT NULL,
    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ,
    fetched_count   INTEGER     NOT NULL DEFAULT 0,
    stored_count    INTEGER     NOT NULL DEFAULT 0,
    error_message   TEXT,
    cursor_snapshot JSONB       NOT NULL DEFAULT '{}'::jsonb
);

-- Platform authors (Plan 003)
CREATE TABLE IF NOT EXISTS platform_authors (
    id                 BIGSERIAL PRIMARY KEY,
    platform           TEXT        NOT NULL,
    platform_author_id TEXT       NOT NULL,
    handle             TEXT        NOT NULL,
    display_name       TEXT        NOT NULL,
    followers_count    INTEGER     NOT NULL DEFAULT 0,
    verified           BOOLEAN     NOT NULL DEFAULT false,
    raw_payload        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(platform, platform_author_id)
);

-- Platform posts (Plan 003)
CREATE TABLE IF NOT EXISTS platform_posts (
    id                BIGSERIAL PRIMARY KEY,
    platform          TEXT        NOT NULL,
    platform_post_id  TEXT        NOT NULL,
    author_platform_id TEXT       NOT NULL,
    author_name       TEXT        NOT NULL,
    author_handle     TEXT        NOT NULL,
    content_text      TEXT        NOT NULL,
    content_lang      TEXT        NOT NULL,
    post_url          TEXT        NOT NULL,
    published_at      TIMESTAMPTZ NOT NULL,
    like_count        INTEGER     NOT NULL DEFAULT 0,
    reply_count       INTEGER     NOT NULL DEFAULT 0,
    repost_count      INTEGER     NOT NULL DEFAULT 0,
    quote_count       INTEGER     NOT NULL DEFAULT 0,
    view_count        INTEGER     NOT NULL DEFAULT 0,
    raw_payload       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    normalized_hash   TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(platform, platform_post_id)
);

CREATE INDEX IF NOT EXISTS idx_platform_posts_normalized_hash ON platform_posts(normalized_hash);
CREATE INDEX IF NOT EXISTS idx_platform_posts_published_at    ON platform_posts(published_at);

-- Monitor post hits (Plan 003)
CREATE TABLE IF NOT EXISTS monitor_post_hits (
    id                   BIGSERIAL PRIMARY KEY,
    monitor_id           BIGINT      NOT NULL REFERENCES keyword_monitors(id),
    post_id              BIGINT      NOT NULL REFERENCES platform_posts(id),
    matched_keywords     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    relevance_score      NUMERIC(5,4)  NOT NULL DEFAULT 0,
    heat_score           NUMERIC(10,4) NOT NULL DEFAULT 0,
    freshness_score      NUMERIC(10,4) NOT NULL DEFAULT 0,
    author_influence_score NUMERIC(10,4) NOT NULL DEFAULT 0,
    final_score          NUMERIC(10,4) NOT NULL DEFAULT 0,
    first_seen_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (jsonb_typeof(matched_keywords) = 'array'),
    UNIQUE(monitor_id, post_id)
);

COMMIT;
