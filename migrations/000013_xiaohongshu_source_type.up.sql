-- Add xiaohongshu to allowed source types
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_type_check;
ALTER TABLE sources ADD CONSTRAINT sources_type_check CHECK (type IN ('rss', 'public_page', 'xiaohongshu'));

-- Add compliance note check for xiaohongshu (similar to public_page)
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_compliance_check;
ALTER TABLE sources ADD CONSTRAINT sources_compliance_check
    CHECK (type NOT IN ('public_page', 'xiaohongshu') OR compliance_note ~ E'\\S');
