-- Add hackernews and wechat_mp to the allowed source types.
-- The original CHECK constraint only allowed 'rss' and 'public_page'.

ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_type_check;
ALTER TABLE sources ADD CONSTRAINT sources_type_check
    CHECK (type IN ('rss', 'public_page', 'hackernews', 'wechat_mp'));

-- Also update the compliance_note check to include wechat_mp.
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_check;
ALTER TABLE sources ADD CONSTRAINT sources_check
    CHECK (type NOT IN ('public_page', 'wechat_mp') OR compliance_note ~ E'\\S');
