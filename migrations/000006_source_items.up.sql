CREATE TABLE IF NOT EXISTS source_items (
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
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (canonical_url),
    CHECK (
        (status = 'primary' AND duplicate_of_item_id IS NULL) OR
        (status = 'duplicate' AND duplicate_of_item_id IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_source_items_source_id_created_at
    ON source_items (source_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_source_items_content_hash
    ON source_items (content_hash);

CREATE INDEX IF NOT EXISTS idx_source_items_status
    ON source_items (status);
