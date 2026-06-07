CREATE TABLE IF NOT EXISTS authorizations (
    id VARCHAR(64) PRIMARY KEY,
    user_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform VARCHAR(32) NOT NULL CHECK (platform IN ('github', 'wechat', 'rss', 'custom')),
    platform_user_id VARCHAR(255) NOT NULL DEFAULT '',
    display_name VARCHAR(255) NOT NULL DEFAULT '',
    access_token_enc TEXT NOT NULL,
    refresh_token_enc TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'connected' CHECK (status IN ('connected', 'expired', 'revoked')),
    connected_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_checked_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE,
    revoked_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, platform)
);

CREATE INDEX idx_authorizations_user_id ON authorizations(user_id);
CREATE INDEX idx_authorizations_platform ON authorizations(platform);
CREATE INDEX idx_authorizations_status ON authorizations(status);
