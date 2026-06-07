ALTER TABLE source_items
    ADD COLUMN IF NOT EXISTS filter_status text NOT NULL DEFAULT 'unknown' CHECK (filter_status IN ('unknown', 'passed', 'filtered')),
    ADD COLUMN IF NOT EXISTS filter_reason text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS quality_score double precision NOT NULL DEFAULT 0.0,
    ADD COLUMN IF NOT EXISTS summarizable boolean NOT NULL DEFAULT false;
