-- Add X platform source type and OAuth state management.

-- Extend sources.type CHECK constraint to include 'x'.
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_type_check;
ALTER TABLE sources ADD CONSTRAINT sources_type_check CHECK (type IN ('rss', 'public_page', 'x'));

-- X sources require compliance notes, same as public_page.
-- Update the existing compliance note constraint to include x.
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_type_compliance_check;
ALTER TABLE sources ADD CONSTRAINT sources_type_compliance_check
    CHECK (type NOT IN ('public_page', 'x') OR compliance_note ~ E'\\S');

-- OAuth state management for X authorization flow.
CREATE TABLE x_oauth_states (
    state text PRIMARY KEY,
    code_verifier text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '10 minutes')
);

CREATE INDEX idx_x_oauth_states_expires_at ON x_oauth_states (expires_at);

-- OAuth credentials for X sources.
CREATE TABLE x_credentials (
    source_id text PRIMARY KEY REFERENCES sources (id) ON DELETE CASCADE,
    access_token text NOT NULL,
    refresh_token text NOT NULL DEFAULT '',
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
