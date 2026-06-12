-- HotKey Server Schema
-- Plans 002-005 table definitions

BEGIN;

-- ============================================================
-- Plan 002: Users & Monitors
-- ============================================================

CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    display_name  TEXT        NOT NULL DEFAULT '',
    status        TEXT        NOT NULL DEFAULT 'active',
    plan_type     TEXT        NOT NULL DEFAULT 'free',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS keyword_monitors (
    id                     BIGSERIAL PRIMARY KEY,
    user_id                BIGINT    NOT NULL REFERENCES users(id),
    name                   TEXT      NOT NULL,
    query_text             TEXT      NOT NULL,
    language               TEXT      NOT NULL DEFAULT 'en',
    region                 TEXT      NOT NULL DEFAULT '',
    status                 TEXT      NOT NULL DEFAULT 'active',
    poll_interval_minutes  INTEGER   NOT NULL DEFAULT 10,
    alert_enabled          BOOLEAN   NOT NULL DEFAULT false,
    alert_threshold_config JSONB     NOT NULL DEFAULT '{}',
    last_polled_at         TIMESTAMPTZ,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_keyword_monitors_user_id ON keyword_monitors(user_id);
CREATE INDEX IF NOT EXISTS idx_keyword_monitors_status  ON keyword_monitors(status);

CREATE TABLE IF NOT EXISTS monitor_runs (
    id              BIGSERIAL PRIMARY KEY,
    monitor_id      BIGINT    NOT NULL REFERENCES keyword_monitors(id),
    platform        TEXT      NOT NULL DEFAULT 'x',
    run_type        TEXT      NOT NULL DEFAULT 'poll',
    status          TEXT      NOT NULL DEFAULT 'pending',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at     TIMESTAMPTZ,
    fetched_count   INTEGER   NOT NULL DEFAULT 0,
    stored_count    INTEGER   NOT NULL DEFAULT 0,
    error_message   TEXT      NOT NULL DEFAULT '',
    cursor_snapshot JSONB     NOT NULL DEFAULT '{}'
);

-- ============================================================
-- Plan 003: Platform Content & Hits
-- ============================================================

CREATE TABLE IF NOT EXISTS platform_posts (
    id                BIGSERIAL PRIMARY KEY,
    platform          TEXT      NOT NULL DEFAULT 'x',
    platform_post_id  TEXT      NOT NULL,
    author_platform_id TEXT     NOT NULL DEFAULT '',
    author_name       TEXT      NOT NULL DEFAULT '',
    author_handle     TEXT      NOT NULL DEFAULT '',
    content_text      TEXT      NOT NULL DEFAULT '',
    content_lang      TEXT      NOT NULL DEFAULT '',
    post_url          TEXT      NOT NULL DEFAULT '',
    published_at      TIMESTAMPTZ,
    like_count        INTEGER   NOT NULL DEFAULT 0,
    reply_count       INTEGER   NOT NULL DEFAULT 0,
    repost_count      INTEGER   NOT NULL DEFAULT 0,
    quote_count       INTEGER   NOT NULL DEFAULT 0,
    view_count        INTEGER   NOT NULL DEFAULT 0,
    raw_payload       JSONB     NOT NULL DEFAULT '{}',
    normalized_hash   TEXT      NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(platform, platform_post_id)
);

CREATE TABLE IF NOT EXISTS platform_authors (
    id                  BIGSERIAL PRIMARY KEY,
    platform            TEXT      NOT NULL DEFAULT 'x',
    platform_author_id  TEXT      NOT NULL,
    handle              TEXT      NOT NULL DEFAULT '',
    display_name        TEXT      NOT NULL DEFAULT '',
    followers_count     INTEGER   NOT NULL DEFAULT 0,
    verified            BOOLEAN   NOT NULL DEFAULT false,
    raw_payload         JSONB     NOT NULL DEFAULT '{}',
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(platform, platform_author_id)
);

CREATE TABLE IF NOT EXISTS monitor_post_hits (
    id                     BIGSERIAL PRIMARY KEY,
    monitor_id             BIGINT      NOT NULL REFERENCES keyword_monitors(id),
    post_id                BIGINT      NOT NULL REFERENCES platform_posts(id),
    matched_keywords       JSONB       NOT NULL DEFAULT '[]',
    relevance_score        NUMERIC(10,4) NOT NULL DEFAULT 0,
    heat_score             NUMERIC(10,4) NOT NULL DEFAULT 0,
    freshness_score        NUMERIC(10,4) NOT NULL DEFAULT 0,
    author_influence_score NUMERIC(10,4) NOT NULL DEFAULT 0,
    final_score            NUMERIC(10,4) NOT NULL DEFAULT 0,
    first_seen_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(monitor_id, post_id)
);

-- ============================================================
-- Plan 004: Topics & Trends
-- ============================================================

CREATE TABLE IF NOT EXISTS topics (
    id                  BIGSERIAL PRIMARY KEY,
    monitor_id          BIGINT      NOT NULL REFERENCES keyword_monitors(id),
    topic_key           TEXT        NOT NULL,
    title               TEXT        NOT NULL,
    summary             TEXT        NOT NULL DEFAULT '',
    status              TEXT        NOT NULL DEFAULT 'active',
    first_detected_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_active_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    current_heat_score  NUMERIC(10,4) NOT NULL DEFAULT 0,
    trend_direction     TEXT        NOT NULL DEFAULT 'flat',
    representative_post_id BIGINT   REFERENCES platform_posts(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(monitor_id, topic_key)
);

CREATE TABLE IF NOT EXISTS topic_posts (
    id                BIGSERIAL PRIMARY KEY,
    topic_id          BIGINT      NOT NULL REFERENCES topics(id),
    post_id           BIGINT      NOT NULL REFERENCES platform_posts(id),
    membership_score  NUMERIC(10,4) NOT NULL DEFAULT 0,
    is_representative BOOLEAN     NOT NULL DEFAULT false,
    added_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(topic_id, post_id)
);

CREATE TABLE IF NOT EXISTS topic_snapshots (
    id                BIGSERIAL PRIMARY KEY,
    topic_id          BIGINT      NOT NULL REFERENCES topics(id),
    snapshot_time     TIMESTAMPTZ NOT NULL,
    post_count        INTEGER     NOT NULL DEFAULT 0,
    unique_author_count INTEGER   NOT NULL DEFAULT 0,
    engagement_sum    INTEGER     NOT NULL DEFAULT 0,
    heat_score        NUMERIC(10,4) NOT NULL DEFAULT 0,
    trend_velocity    NUMERIC(10,4) NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS monitor_snapshots (
    id                 BIGSERIAL PRIMARY KEY,
    monitor_id         BIGINT      NOT NULL REFERENCES keyword_monitors(id),
    snapshot_time      TIMESTAMPTZ NOT NULL,
    new_post_count     INTEGER     NOT NULL DEFAULT 0,
    active_topic_count INTEGER     NOT NULL DEFAULT 0,
    total_engagement   INTEGER     NOT NULL DEFAULT 0,
    top_topic_id       BIGINT      REFERENCES topics(id)
);

-- ============================================================
-- Plan 005: Alerts & Notifications (placeholder)
-- ============================================================

CREATE TABLE IF NOT EXISTS alerts (
    id             BIGSERIAL PRIMARY KEY,
    monitor_id     BIGINT      NOT NULL REFERENCES keyword_monitors(id),
    topic_id       BIGINT      REFERENCES topics(id),
    alert_type     TEXT        NOT NULL DEFAULT 'threshold',
    title          TEXT        NOT NULL,
    message        TEXT        NOT NULL DEFAULT '',
    severity       TEXT        NOT NULL DEFAULT 'info',
    trigger_score  NUMERIC(10,4) NOT NULL DEFAULT 0,
    trigger_reason TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS user_notifications (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT      NOT NULL REFERENCES users(id),
    alert_id        BIGINT      NOT NULL REFERENCES alerts(id),
    channel         TEXT        NOT NULL DEFAULT 'in_app',
    delivery_status TEXT        NOT NULL DEFAULT 'pending',
    read_at         TIMESTAMPTZ,
    sent_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS email_deliveries (
    id                BIGSERIAL PRIMARY KEY,
    notification_id   BIGINT      NOT NULL REFERENCES user_notifications(id),
    recipient_email   TEXT        NOT NULL,
    provider          TEXT        NOT NULL DEFAULT '',
    provider_message_id TEXT      NOT NULL DEFAULT '',
    status            TEXT        NOT NULL DEFAULT 'pending',
    error_message     TEXT        NOT NULL DEFAULT '',
    sent_at           TIMESTAMPTZ
);

COMMIT;
