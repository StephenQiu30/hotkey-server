-- Revert xiaohongshu source type
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_compliance_check;
ALTER TABLE sources ADD CONSTRAINT sources_compliance_check
    CHECK (type <> 'public_page' OR compliance_note ~ E'\\S');

ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_type_check;
ALTER TABLE sources ADD CONSTRAINT sources_type_check CHECK (type IN ('rss', 'public_page'));
