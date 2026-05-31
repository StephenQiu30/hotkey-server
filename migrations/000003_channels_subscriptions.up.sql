CREATE TABLE IF NOT EXISTS channels (
    id text PRIMARY KEY,
    name text NOT NULL,
    slug text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_channels_status ON channels (status);

INSERT INTO channels (id, name, slug, description, status)
VALUES
    ('chn_ai_models', 'AI 模型', 'ai-models', 'AI 模型发布、能力更新与评测', 'active'),
    ('chn_ai_products', 'AI 产品', 'ai-products', 'AI 产品发布、增长与使用场景', 'active'),
    ('chn_ai_open_source', 'AI 开源', 'ai-open-source', 'AI 开源项目、框架与社区动态', 'active'),
    ('chn_ai_funding', 'AI 投融资', 'ai-funding', 'AI 公司融资、并购与资本动态', 'active')
ON CONFLICT (id) DO UPDATE SET
    name = EXCLUDED.name,
    slug = EXCLUDED.slug,
    description = EXCLUDED.description,
    status = EXCLUDED.status,
    updated_at = now();

CREATE TABLE IF NOT EXISTS user_channel_subscriptions (
    user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    channel_id text NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, channel_id)
);

CREATE INDEX IF NOT EXISTS idx_user_channel_subscriptions_channel_id
    ON user_channel_subscriptions (channel_id);

CREATE TABLE IF NOT EXISTS user_keywords (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    keyword text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_user_keywords_user_id ON user_keywords (user_id);
CREATE INDEX IF NOT EXISTS idx_user_keywords_enabled ON user_keywords (enabled);

CREATE TABLE IF NOT EXISTS system_settings (
    key text PRIMARY KEY,
    value text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO system_settings (key, value)
VALUES ('default_daily_send_at', '08:30')
ON CONFLICT (key) DO NOTHING;
