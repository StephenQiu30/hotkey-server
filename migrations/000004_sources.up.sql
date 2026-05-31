CREATE TABLE sources (
    id text PRIMARY KEY,
    name text NOT NULL,
    type text NOT NULL CHECK (type IN ('rss', 'public_page')),
    url text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled', 'disabled')),
    compliance_note text NOT NULL DEFAULT '',
    fetch_interval_min integer NOT NULL CHECK (fetch_interval_min > 0),
    rate_limit_per_hour integer NOT NULL DEFAULT 0 CHECK (rate_limit_per_hour >= 0),
    last_error text NOT NULL DEFAULT '',
    last_collected_at timestamptz,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK (type <> 'public_page' OR compliance_note ~ E'\\S')
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
