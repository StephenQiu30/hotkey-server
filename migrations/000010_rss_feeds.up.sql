CREATE TABLE IF NOT EXISTS rss_feeds (
    user_id text NOT NULL PRIMARY KEY,
    token_hash text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    last_accessed_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (token_hash)
);
