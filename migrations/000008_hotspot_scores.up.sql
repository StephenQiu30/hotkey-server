CREATE TABLE IF NOT EXISTS hotspot_scores (
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

CREATE INDEX IF NOT EXISTS idx_hotspot_scores_cluster_id
    ON hotspot_scores (cluster_id);

CREATE INDEX IF NOT EXISTS idx_hotspot_scores_total_score
    ON hotspot_scores (total_score DESC);
