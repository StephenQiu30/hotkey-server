-- E2E test seed data.
-- Loaded automatically by docker-compose.e2e.yml into PostgreSQL on first start.

-- Tenant
INSERT INTO tenants (id, name, created_at, updated_at)
VALUES
  ('t_e2e_001', 'E2E 测试租户', now(), now())
ON CONFLICT DO NOTHING;

-- Users
INSERT INTO users (id, tenant_id, name, email, role, created_at, updated_at)
VALUES
  ('u_e2e_001', 't_e2e_001', '测试管理员', 'admin@e2e.test', 'admin', now(), now()),
  ('u_e2e_002', 't_e2e_001', '测试编辑', 'editor@e2e.test', 'editor', now(), now())
ON CONFLICT DO NOTHING;

-- Keywords
INSERT INTO keywords (id, tenant_id, word, category, enabled, created_at, updated_at)
VALUES
  ('k_e2e_001', 't_e2e_001', 'AI', 'technology', true, now(), now()),
  ('k_e2e_002', 't_e2e_001', '大模型', 'technology', true, now(), now()),
  ('k_e2e_003', 't_e2e_001', '数字化转型', 'business', true, now(), now())
ON CONFLICT DO NOTHING;

-- Sources
INSERT INTO sources (id, tenant_id, name, type, url, enabled, created_at, updated_at)
VALUES
  ('s_e2e_001', 't_e2e_001', '测试 RSS 源', 'rss', 'https://example.com/feed.xml', true, now(), now()),
  ('s_e2e_002', 't_e2e_001', '测试网页源', 'web', 'https://example.com/news', true, now(), now())
ON CONFLICT DO NOTHING;

-- Contents
INSERT INTO contents (id, tenant_id, source_id, title, body, url, published_at, created_at)
VALUES
  ('c_e2e_001', 't_e2e_001', 's_e2e_001', 'AI 技术突破：新一代大模型发布', '多家科技公司发布最新 AI 模型。', 'https://example.com/article/1', now(), now()),
  ('c_e2e_002', 't_e2e_001', 's_e2e_001', '数字化转型加速推进', '报告显示数字化转型投入持续增长。', 'https://example.com/article/2', now(), now())
ON CONFLICT DO NOTHING;

-- Events
INSERT INTO events (id, tenant_id, keyword_id, content_id, score, created_at)
VALUES
  ('e_e2e_001', 't_e2e_001', 'k_e2e_001', 'c_e2e_001', 0.95, now()),
  ('e_e2e_002', 't_e2e_001', 'k_e2e_003', 'c_e2e_002', 0.80, now())
ON CONFLICT DO NOTHING;
