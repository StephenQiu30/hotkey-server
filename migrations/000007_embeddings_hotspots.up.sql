CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS item_embeddings (
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

CREATE INDEX IF NOT EXISTS idx_item_embeddings_status_updated_at
    ON item_embeddings (status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_item_embeddings_embedding
    ON item_embeddings USING hnsw (embedding vector_cosine_ops)
    WHERE status = 'succeeded';

CREATE TABLE IF NOT EXISTS hotspot_clusters (
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

CREATE INDEX IF NOT EXISTS idx_hotspot_clusters_window
    ON hotspot_clusters (window_start, window_end);

CREATE INDEX IF NOT EXISTS idx_hotspot_clusters_centroid
    ON hotspot_clusters USING hnsw (centroid vector_cosine_ops);

CREATE TABLE IF NOT EXISTS hotspot_items (
    cluster_id text NOT NULL REFERENCES hotspot_clusters (id) ON DELETE CASCADE,
    item_id text NOT NULL REFERENCES source_items (id) ON DELETE CASCADE,
    similarity double precision NOT NULL CHECK (similarity >= -1 AND similarity <= 1),
    created_at timestamptz NOT NULL,
    PRIMARY KEY (cluster_id, item_id)
);

CREATE INDEX IF NOT EXISTS idx_hotspot_items_item_id
    ON hotspot_items (item_id);
