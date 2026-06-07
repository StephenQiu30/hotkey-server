-- Remove bilibili and hackernews from sources.type CHECK constraint
-- First delete any rows using the removed types to avoid CHECK violation
DELETE FROM sources WHERE type IN ('bilibili', 'hackernews');
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_type_check;
ALTER TABLE sources ADD CONSTRAINT sources_type_check
    CHECK (type IN ('rss', 'public_page'));
