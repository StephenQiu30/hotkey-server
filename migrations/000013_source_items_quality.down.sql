ALTER TABLE source_items
    DROP COLUMN IF EXISTS filter_status,
    DROP COLUMN IF EXISTS filter_reason,
    DROP COLUMN IF EXISTS quality_score,
    DROP COLUMN IF EXISTS summarizable;
