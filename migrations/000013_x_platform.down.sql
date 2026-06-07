-- Revert X platform additions.

DROP TABLE IF EXISTS x_credentials;
DROP TABLE IF EXISTS x_oauth_states;

-- Restore original compliance note constraint (public_page only).
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_type_compliance_check;
ALTER TABLE sources ADD CONSTRAINT sources_type_compliance_check
    CHECK (type <> 'public_page' OR compliance_note ~ E'\\S');

-- Restore original type constraint (rss and public_page only).
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_type_check;
ALTER TABLE sources ADD CONSTRAINT sources_type_check CHECK (type IN ('rss', 'public_page'));
