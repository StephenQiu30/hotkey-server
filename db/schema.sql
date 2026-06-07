-- HotKey Server V1 database schema.
-- Fact source: docs/prd/001-产品总览与上线门禁.md and related V1 PRDs.
-- Target database: PostgreSQL 15+ with pgcrypto, citext, and pgvector.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE OR REPLACE FUNCTION hotkey_touch_updated_at()
RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- Accounts, sessions, authorization custody
-- ---------------------------------------------------------------------------

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email citext NOT NULL UNIQUE,
  display_name text,
  password_hash text,
  status text NOT NULL DEFAULT 'pending_email_verification'
    CHECK (status IN ('pending_email_verification', 'active', 'locked', 'deleting', 'deleted')),
  timezone text NOT NULL DEFAULT 'Asia/Shanghai',
  locale text NOT NULL DEFAULT 'zh-CN',
  email_verified_at timestamptz,
  last_login_at timestamptz,
  delete_requested_at timestamptz,
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE users IS '个人创作者 / 研究者账号。V1 不做团队或组织空间。';
COMMENT ON COLUMN users.status IS 'pending_email_verification/active/locked/deleting/deleted';

CREATE TRIGGER users_touch_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE user_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  session_token_hash text NOT NULL UNIQUE,
  status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'revoked', 'expired')),
  ip_address inet,
  user_agent text,
  last_seen_at timestamptz,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX user_sessions_user_id_idx ON user_sessions(user_id);
CREATE INDEX user_sessions_expires_at_idx ON user_sessions(expires_at);

CREATE TRIGGER user_sessions_touch_updated_at
BEFORE UPDATE ON user_sessions
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE platform_providers (
  id text PRIMARY KEY,
  display_name text NOT NULL,
  provider_kind text NOT NULL
    CHECK (provider_kind IN ('social', 'video', 'community', 'rss', 'news', 'git', 'email', 'ai')),
  auth_required boolean NOT NULL DEFAULT true,
  status text NOT NULL DEFAULT 'available'
    CHECK (status IN ('available', 'degraded', 'limited', 'disabled')),
  capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
  compliance_note text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE platform_providers IS '平台能力矩阵。包含 X、Reddit、YouTube、HN、微博、小红书、知乎、B站、微信公众号/RSS、新闻/RSS、Git、邮件等。';

CREATE TRIGGER platform_providers_touch_updated_at
BEFORE UPDATE ON platform_providers
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

INSERT INTO platform_providers (id, display_name, provider_kind, auth_required, capabilities)
VALUES
  ('x', 'X', 'social', true, '{"keywords": true, "engagement": true}'::jsonb),
  ('reddit', 'Reddit', 'community', true, '{"keywords": true, "comments": true, "engagement": true}'::jsonb),
  ('youtube', 'YouTube', 'video', true, '{"keywords": true, "channels": true, "transcript": true}'::jsonb),
  ('hacker_news', 'Hacker News', 'community', false, '{"public_feed": true, "comments": true}'::jsonb),
  ('weibo', '微博', 'social', true, '{"keywords": true, "engagement": true}'::jsonb),
  ('xiaohongshu', '小红书', 'social', true, '{"keywords": true, "notes": true}'::jsonb),
  ('zhihu', '知乎', 'community', true, '{"questions": true, "answers": true, "articles": true}'::jsonb),
  ('bilibili', 'B站', 'video', true, '{"videos": true, "dynamics": true, "articles": true}'::jsonb),
  ('wechat_rss', '微信公众号/RSS', 'rss', false, '{"feeds": true, "articles": true}'::jsonb),
  ('news_rss', '新闻/RSS', 'news', false, '{"feeds": true, "articles": true}'::jsonb),
  ('github', 'GitHub', 'git', true, '{"repository_sync": true}'::jsonb),
  ('smtp', 'SMTP', 'email', true, '{"report_delivery": true}'::jsonb),
  ('ai', 'AI Provider', 'ai', true, '{"summary": true, "embedding": true}'::jsonb)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE authorization_connections (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider_id text NOT NULL REFERENCES platform_providers(id),
  connection_kind text NOT NULL DEFAULT 'oauth'
    CHECK (connection_kind IN ('oauth', 'api_key', 'cookie', 'rss', 'public', 'git_token', 'smtp', 'ai_key')),
  status text NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft', 'connected', 'refreshing', 'limited', 'expired', 'revoked', 'failed')),
  external_account_id text,
  external_account_name text,
  scopes text[] NOT NULL DEFAULT '{}',
  rate_limit jsonb NOT NULL DEFAULT '{}'::jsonb,
  last_health_check_at timestamptz,
  last_success_at timestamptz,
  last_failure_at timestamptz,
  failure_category text
    CHECK (failure_category IS NULL OR failure_category IN ('auth', 'rate_limit', 'network', 'provider_schema', 'content_unavailable', 'policy_restricted', 'unknown')),
  failure_reason text,
  expires_at timestamptz,
  revoked_at timestamptz,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, provider_id, external_account_id)
);

COMMENT ON TABLE authorization_connections IS '用户自备平台账号、Git、SMTP 和 AI provider 授权连接。';

CREATE INDEX authorization_connections_user_idx ON authorization_connections(user_id);
CREATE INDEX authorization_connections_provider_status_idx ON authorization_connections(provider_id, status);

CREATE TRIGGER authorization_connections_touch_updated_at
BEFORE UPDATE ON authorization_connections
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE credential_secrets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  connection_id uuid REFERENCES authorization_connections(id) ON DELETE CASCADE,
  secret_kind text NOT NULL
    CHECK (secret_kind IN ('access_token', 'refresh_token', 'api_key', 'cookie', 'smtp_password', 'git_token', 'ai_key')),
  encrypted_payload bytea NOT NULL,
  encryption_key_id text NOT NULL,
  payload_checksum text NOT NULL,
  status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'rotated', 'revoked', 'deleted')),
  expires_at timestamptz,
  rotated_at timestamptz,
  revoked_at timestamptz,
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE credential_secrets IS '敏感凭据密文仓。禁止存储或记录明文 token。';

CREATE INDEX credential_secrets_user_idx ON credential_secrets(user_id);
CREATE INDEX credential_secrets_connection_idx ON credential_secrets(connection_id);

CREATE TRIGGER credential_secrets_touch_updated_at
BEFORE UPDATE ON credential_secrets
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

-- ---------------------------------------------------------------------------
-- Topics, sources, scheduling
-- ---------------------------------------------------------------------------

CREATE TABLE monitored_topics (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name text NOT NULL,
  description text,
  semantic_prompt text,
  include_keywords text[] NOT NULL DEFAULT '{}',
  exclude_keywords text[] NOT NULL DEFAULT '{}',
  languages text[] NOT NULL DEFAULT '{}',
  regions text[] NOT NULL DEFAULT '{}',
  status text NOT NULL DEFAULT 'draft'
    CHECK (status IN ('draft', 'active', 'paused', 'invalid', 'deleting', 'deleted')),
  similarity_threshold numeric(5,4) NOT NULL DEFAULT 0.7800 CHECK (similarity_threshold >= 0 AND similarity_threshold <= 1),
  heat_threshold numeric(8,4) NOT NULL DEFAULT 0,
  collection_frequency_minutes integer NOT NULL DEFAULT 60 CHECK (collection_frequency_minutes >= 5),
  obsidian_folder text,
  report_enabled boolean NOT NULL DEFAULT true,
  last_collected_at timestamptz,
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, name)
);

COMMENT ON TABLE monitored_topics IS '用户监控主题与关键词配置。';

CREATE INDEX monitored_topics_user_status_idx ON monitored_topics(user_id, status);

CREATE TRIGGER monitored_topics_touch_updated_at
BEFORE UPDATE ON monitored_topics
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE topic_sources (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  topic_id uuid NOT NULL REFERENCES monitored_topics(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider_id text NOT NULL REFERENCES platform_providers(id),
  connection_id uuid REFERENCES authorization_connections(id) ON DELETE SET NULL,
  status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'paused', 'auth_required', 'limited', 'failed', 'deleted')),
  config jsonb NOT NULL DEFAULT '{}'::jsonb,
  last_collected_at timestamptz,
  last_failure_at timestamptz,
  failure_reason text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (topic_id, provider_id, connection_id)
);

CREATE INDEX topic_sources_topic_idx ON topic_sources(topic_id);
CREATE INDEX topic_sources_user_provider_idx ON topic_sources(user_id, provider_id, status);

CREATE TRIGGER topic_sources_touch_updated_at
BEFORE UPDATE ON topic_sources
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE feed_sources (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider_id text NOT NULL REFERENCES platform_providers(id),
  url text NOT NULL,
  title text,
  source_kind text NOT NULL
    CHECK (source_kind IN ('rss', 'wechat_public_article', 'news_rss', 'web_page')),
  status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'invalid_url', 'fetch_failed', 'parse_failed', 'stale', 'disabled', 'deleted')),
  last_fetched_at timestamptz,
  last_success_at timestamptz,
  last_failure_at timestamptz,
  failure_reason text,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  deleted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, provider_id, url)
);

CREATE INDEX feed_sources_user_status_idx ON feed_sources(user_id, status);

CREATE TRIGGER feed_sources_touch_updated_at
BEFORE UPDATE ON feed_sources
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE collection_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  topic_id uuid REFERENCES monitored_topics(id) ON DELETE SET NULL,
  topic_source_id uuid REFERENCES topic_sources(id) ON DELETE SET NULL,
  provider_id text NOT NULL REFERENCES platform_providers(id),
  connection_id uuid REFERENCES authorization_connections(id) ON DELETE SET NULL,
  feed_source_id uuid REFERENCES feed_sources(id) ON DELETE SET NULL,
  status text NOT NULL DEFAULT 'queued'
    CHECK (status IN ('queued', 'running', 'rate_limited', 'auth_failed', 'provider_failed', 'normalized', 'empty', 'completed', 'failed', 'cancelled')),
  idempotency_key text NOT NULL UNIQUE,
  scheduled_for timestamptz NOT NULL DEFAULT now(),
  window_start timestamptz,
  window_end timestamptz,
  started_at timestamptz,
  completed_at timestamptz,
  attempt_count integer NOT NULL DEFAULT 0,
  max_attempts integer NOT NULL DEFAULT 3,
  next_retry_at timestamptz,
  failure_category text
    CHECK (failure_category IS NULL OR failure_category IN ('auth', 'rate_limit', 'network', 'provider_schema', 'content_unavailable', 'policy_restricted', 'unknown')),
  failure_reason text,
  stats jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX collection_jobs_due_idx ON collection_jobs(status, scheduled_for);
CREATE INDEX collection_jobs_user_topic_idx ON collection_jobs(user_id, topic_id, created_at DESC);
CREATE INDEX collection_jobs_provider_status_idx ON collection_jobs(provider_id, status);

CREATE TRIGGER collection_jobs_touch_updated_at
BEFORE UPDATE ON collection_jobs
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

-- ---------------------------------------------------------------------------
-- Object storage, raw and normalized content
-- ---------------------------------------------------------------------------

CREATE TABLE storage_objects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  bucket text NOT NULL,
  object_key text NOT NULL,
  object_kind text NOT NULL
    CHECK (object_kind IN ('raw_response', 'content_snapshot', 'media_ref', 'markdown_export', 'email_archive', 'test_fixture')),
  content_type text,
  provider_id text REFERENCES platform_providers(id),
  source_platform_item_id text,
  retention_policy text NOT NULL
    CHECK (retention_policy IN ('raw_30_days', 'short_cache_7_days', 'derived_long_term', 'audit_required')),
  status text NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'scheduled_for_expiry', 'expired', 'delete_pending', 'deleted', 'delete_failed')),
  checksum text,
  byte_size bigint,
  expires_at timestamptz,
  delete_requested_at timestamptz,
  deleted_at timestamptz,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (bucket, object_key)
);

COMMENT ON TABLE storage_objects IS 'MinIO 对象索引。原始内容默认 raw_30_days。';

CREATE INDEX storage_objects_user_status_idx ON storage_objects(user_id, status);
CREATE INDEX storage_objects_expires_idx ON storage_objects(status, expires_at);

CREATE TRIGGER storage_objects_touch_updated_at
BEFORE UPDATE ON storage_objects
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE source_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider_id text NOT NULL REFERENCES platform_providers(id),
  connection_id uuid REFERENCES authorization_connections(id) ON DELETE SET NULL,
  collection_job_id uuid REFERENCES collection_jobs(id) ON DELETE SET NULL,
  feed_source_id uuid REFERENCES feed_sources(id) ON DELETE SET NULL,
  raw_object_id uuid REFERENCES storage_objects(id) ON DELETE SET NULL,
  source_item_id text NOT NULL,
  canonical_url text,
  author_name text,
  author_handle text,
  title text,
  body_text text,
  language text,
  content_kind text NOT NULL DEFAULT 'post'
    CHECK (content_kind IN ('post', 'comment', 'article', 'video', 'dynamic', 'rss_item', 'news_article')),
  visibility text NOT NULL DEFAULT 'public'
    CHECK (visibility IN ('public', 'limited', 'deleted', 'unavailable', 'metadata_only')),
  published_at timestamptz,
  fetched_at timestamptz NOT NULL DEFAULT now(),
  engagement jsonb NOT NULL DEFAULT '{}'::jsonb,
  media_refs jsonb NOT NULL DEFAULT '[]'::jsonb,
  license_hint text,
  content_hash text NOT NULL,
  dedupe_key text,
  status text NOT NULL DEFAULT 'normalized'
    CHECK (status IN ('raw_received', 'normalized', 'filtered_out', 'similarity_matched', 'duplicate', 'ready_for_clustering', 'processing_failed')),
  failure_reason text,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, provider_id, source_item_id)
);

CREATE INDEX source_items_user_provider_idx ON source_items(user_id, provider_id, fetched_at DESC);
CREATE INDEX source_items_canonical_url_idx ON source_items(canonical_url) WHERE canonical_url IS NOT NULL;
CREATE INDEX source_items_content_hash_idx ON source_items(user_id, content_hash);
CREATE INDEX source_items_status_idx ON source_items(status, fetched_at DESC);

CREATE TRIGGER source_items_touch_updated_at
BEFORE UPDATE ON source_items
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE topic_content_matches (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  topic_id uuid NOT NULL REFERENCES monitored_topics(id) ON DELETE CASCADE,
  source_item_id uuid NOT NULL REFERENCES source_items(id) ON DELETE CASCADE,
  match_status text NOT NULL DEFAULT 'pending'
    CHECK (match_status IN ('pending', 'filtered_out', 'similarity_matched', 'duplicate', 'ready_for_clustering', 'failed')),
  keyword_score numeric(6,5),
  similarity_score numeric(6,5),
  quality_score numeric(6,5),
  filter_reason text,
  matched_keywords text[] NOT NULL DEFAULT '{}',
  duplicate_of_source_item_id uuid REFERENCES source_items(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (topic_id, source_item_id)
);

CREATE INDEX topic_content_matches_topic_status_idx ON topic_content_matches(topic_id, match_status);
CREATE INDEX topic_content_matches_source_idx ON topic_content_matches(source_item_id);

CREATE TRIGGER topic_content_matches_touch_updated_at
BEFORE UPDATE ON topic_content_matches
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE content_dedupe_groups (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  topic_id uuid REFERENCES monitored_topics(id) ON DELETE CASCADE,
  primary_source_item_id uuid REFERENCES source_items(id) ON DELETE SET NULL,
  dedupe_type text NOT NULL
    CHECK (dedupe_type IN ('source_id', 'canonical_url', 'text_hash', 'near_duplicate')),
  dedupe_key text NOT NULL,
  member_count integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, topic_id, dedupe_type, dedupe_key)
);

CREATE TABLE content_dedupe_members (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  dedupe_group_id uuid NOT NULL REFERENCES content_dedupe_groups(id) ON DELETE CASCADE,
  source_item_id uuid NOT NULL REFERENCES source_items(id) ON DELETE CASCADE,
  similarity_score numeric(6,5),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (dedupe_group_id, source_item_id)
);

CREATE TRIGGER content_dedupe_groups_touch_updated_at
BEFORE UPDATE ON content_dedupe_groups
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE content_embeddings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source_item_id uuid NOT NULL REFERENCES source_items(id) ON DELETE CASCADE,
  topic_id uuid REFERENCES monitored_topics(id) ON DELETE CASCADE,
  embedding_model text NOT NULL,
  embedding_dimensions integer NOT NULL DEFAULT 1536,
  content_hash text NOT NULL,
  embedding vector(1536) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_item_id, embedding_model, content_hash)
);

CREATE INDEX content_embeddings_user_topic_idx ON content_embeddings(user_id, topic_id);
CREATE INDEX content_embeddings_vector_idx ON content_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- ---------------------------------------------------------------------------
-- Hotspot events, AI summaries
-- ---------------------------------------------------------------------------

CREATE TABLE hotspot_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  topic_id uuid NOT NULL REFERENCES monitored_topics(id) ON DELETE CASCADE,
  status text NOT NULL DEFAULT 'forming'
    CHECK (status IN ('forming', 'active', 'updated', 'cooling', 'archived', 'suppressed')),
  title text NOT NULL,
  short_summary text,
  evidence_state text NOT NULL DEFAULT 'single_source'
    CHECK (evidence_state IN ('sufficient', 'single_source', 'low_confidence', 'conflicting')),
  heat_score numeric(8,4) NOT NULL DEFAULT 0,
  relevance_score numeric(6,5) NOT NULL DEFAULT 0,
  credibility_score numeric(6,5) NOT NULL DEFAULT 0,
  freshness_score numeric(6,5) NOT NULL DEFAULT 0,
  trend_score numeric(8,4) NOT NULL DEFAULT 0,
  first_seen_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  last_scored_at timestamptz,
  archived_at timestamptz,
  suppressed_at timestamptz,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX hotspot_events_topic_status_idx ON hotspot_events(topic_id, status, heat_score DESC);
CREATE INDEX hotspot_events_user_seen_idx ON hotspot_events(user_id, last_seen_at DESC);

CREATE TRIGGER hotspot_events_touch_updated_at
BEFORE UPDATE ON hotspot_events
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE event_sources (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
  source_item_id uuid NOT NULL REFERENCES source_items(id) ON DELETE CASCADE,
  evidence_role text NOT NULL DEFAULT 'supporting'
    CHECK (evidence_role IN ('primary', 'supporting', 'conflicting', 'context')),
  confidence_score numeric(6,5),
  added_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (event_id, source_item_id)
);

CREATE INDEX event_sources_event_idx ON event_sources(event_id);
CREATE INDEX event_sources_source_idx ON event_sources(source_item_id);

CREATE TABLE event_embeddings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
  embedding_model text NOT NULL,
  embedding_dimensions integer NOT NULL DEFAULT 1536,
  summary_hash text NOT NULL,
  embedding vector(1536) NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (event_id, embedding_model, summary_hash)
);

CREATE INDEX event_embeddings_user_idx ON event_embeddings(user_id);
CREATE INDEX event_embeddings_vector_idx ON event_embeddings USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

CREATE TABLE ai_event_summaries (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
  status text NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'generating', 'generated', 'needs_refresh', 'failed', 'suppressed')),
  model_provider text NOT NULL,
  model_name text NOT NULL,
  prompt_version text NOT NULL,
  output_schema_version text NOT NULL,
  summary_json jsonb,
  source_item_ids uuid[] NOT NULL DEFAULT '{}',
  failure_reason text,
  generated_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (event_id, prompt_version, output_schema_version)
);

CREATE INDEX ai_event_summaries_event_status_idx ON ai_event_summaries(event_id, status);

CREATE TRIGGER ai_event_summaries_touch_updated_at
BEFORE UPDATE ON ai_event_summaries
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

-- ---------------------------------------------------------------------------
-- Obsidian Git sync and email reports
-- ---------------------------------------------------------------------------

CREATE TABLE obsidian_vaults (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  git_connection_id uuid NOT NULL REFERENCES authorization_connections(id) ON DELETE RESTRICT,
  provider text NOT NULL DEFAULT 'github',
  repository_owner text NOT NULL,
  repository_name text NOT NULL,
  branch_name text NOT NULL DEFAULT 'main',
  vault_path text NOT NULL,
  status text NOT NULL DEFAULT 'connected'
    CHECK (status IN ('connected', 'expired', 'permission_limited', 'revoked', 'failed')),
  last_commit_sha text,
  last_synced_at timestamptz,
  failure_reason text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, provider, repository_owner, repository_name, branch_name, vault_path)
);

CREATE INDEX obsidian_vaults_user_status_idx ON obsidian_vaults(user_id, status);

CREATE TRIGGER obsidian_vaults_touch_updated_at
BEFORE UPDATE ON obsidian_vaults
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE git_sync_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  vault_id uuid NOT NULL REFERENCES obsidian_vaults(id) ON DELETE CASCADE,
  event_id uuid REFERENCES hotspot_events(id) ON DELETE SET NULL,
  report_run_id uuid,
  sync_kind text NOT NULL
    CHECK (sync_kind IN ('event_note', 'daily_index', 'weekly_index', 'test')),
  status text NOT NULL DEFAULT 'queued'
    CHECK (status IN ('queued', 'rendering', 'committing', 'conflicted', 'completed', 'failed', 'cancelled')),
  idempotency_key text NOT NULL UNIQUE,
  file_path text NOT NULL,
  markdown_object_id uuid REFERENCES storage_objects(id) ON DELETE SET NULL,
  commit_sha text,
  commit_url text,
  failure_reason text,
  attempt_count integer NOT NULL DEFAULT 0,
  next_retry_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX git_sync_jobs_user_status_idx ON git_sync_jobs(user_id, status, created_at DESC);
CREATE INDEX git_sync_jobs_event_idx ON git_sync_jobs(event_id);

CREATE TRIGGER git_sync_jobs_touch_updated_at
BEFORE UPDATE ON git_sync_jobs
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE report_subscriptions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  report_type text NOT NULL CHECK (report_type IN ('daily', 'weekly')),
  status text NOT NULL DEFAULT 'enabled'
    CHECK (status IN ('enabled', 'disabled', 'invalid_email', 'paused_due_to_bounce')),
  timezone text NOT NULL DEFAULT 'Asia/Shanghai',
  send_time_local time NOT NULL DEFAULT '08:30',
  topic_ids uuid[] NOT NULL DEFAULT '{}',
  recipient_email citext NOT NULL,
  last_sent_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_id, report_type, recipient_email)
);

CREATE INDEX report_subscriptions_due_idx ON report_subscriptions(status, report_type, send_time_local);

CREATE TRIGGER report_subscriptions_touch_updated_at
BEFORE UPDATE ON report_subscriptions
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE report_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  subscription_id uuid REFERENCES report_subscriptions(id) ON DELETE SET NULL,
  report_type text NOT NULL CHECK (report_type IN ('daily', 'weekly')),
  status text NOT NULL DEFAULT 'scheduled'
    CHECK (status IN ('scheduled', 'rendering', 'sending', 'sent', 'retrying', 'failed', 'skipped_no_content')),
  period_start timestamptz NOT NULL,
  period_end timestamptz NOT NULL,
  idempotency_key text NOT NULL UNIQUE,
  rendered_object_id uuid REFERENCES storage_objects(id) ON DELETE SET NULL,
  event_count integer NOT NULL DEFAULT 0,
  failure_reason text,
  sent_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX report_runs_user_period_idx ON report_runs(user_id, report_type, period_start DESC);
CREATE INDEX report_runs_status_idx ON report_runs(status, created_at DESC);

CREATE TRIGGER report_runs_touch_updated_at
BEFORE UPDATE ON report_runs
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

ALTER TABLE git_sync_jobs
  ADD CONSTRAINT git_sync_jobs_report_run_fk
  FOREIGN KEY (report_run_id) REFERENCES report_runs(id) ON DELETE SET NULL;

CREATE TABLE report_event_items (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  report_run_id uuid NOT NULL REFERENCES report_runs(id) ON DELETE CASCADE,
  event_id uuid NOT NULL REFERENCES hotspot_events(id) ON DELETE CASCADE,
  rank_position integer NOT NULL,
  section text NOT NULL DEFAULT 'hot',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (report_run_id, event_id)
);

CREATE TABLE email_deliveries (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  report_run_id uuid REFERENCES report_runs(id) ON DELETE SET NULL,
  provider_id text REFERENCES platform_providers(id),
  recipient_email citext NOT NULL,
  status text NOT NULL DEFAULT 'scheduled'
    CHECK (status IN ('scheduled', 'rendering', 'sending', 'sent', 'retrying', 'failed', 'skipped_no_content')),
  provider_message_id text,
  attempt_count integer NOT NULL DEFAULT 0,
  last_attempt_at timestamptz,
  next_retry_at timestamptz,
  failure_reason text,
  sent_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX email_deliveries_user_status_idx ON email_deliveries(user_id, status, created_at DESC);
CREATE INDEX email_deliveries_report_idx ON email_deliveries(report_run_id);

CREATE TRIGGER email_deliveries_touch_updated_at
BEFORE UPDATE ON email_deliveries
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

-- ---------------------------------------------------------------------------
-- Audit, revocation, deletion, cleanup
-- ---------------------------------------------------------------------------

CREATE TABLE audit_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  actor_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
  event_type text NOT NULL,
  resource_type text NOT NULL,
  resource_id uuid,
  ip_address inet,
  user_agent text,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE audit_events IS '用户可见和运维可追踪审计事件。metadata 禁止写入明文凭据。';

CREATE INDEX audit_events_user_time_idx ON audit_events(user_id, created_at DESC);
CREATE INDEX audit_events_resource_idx ON audit_events(resource_type, resource_id);

CREATE TABLE deletion_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  request_type text NOT NULL
    CHECK (request_type IN ('account', 'topic', 'connection', 'feed_source', 'event')),
  resource_type text NOT NULL,
  resource_id uuid,
  status text NOT NULL DEFAULT 'queued'
    CHECK (status IN ('queued', 'running', 'blocked_by_retry', 'partially_failed', 'completed')),
  requested_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  completed_at timestamptz,
  failure_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX deletion_requests_user_status_idx ON deletion_requests(user_id, status, requested_at DESC);

CREATE TRIGGER deletion_requests_touch_updated_at
BEFORE UPDATE ON deletion_requests
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

CREATE TABLE cleanup_tasks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  deletion_request_id uuid REFERENCES deletion_requests(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  target_type text NOT NULL,
  target_id uuid,
  status text NOT NULL DEFAULT 'queued'
    CHECK (status IN ('queued', 'running', 'blocked_by_retry', 'partially_failed', 'completed', 'failed')),
  attempt_count integer NOT NULL DEFAULT 0,
  max_attempts integer NOT NULL DEFAULT 5,
  next_retry_at timestamptz,
  failure_reason text,
  completed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cleanup_tasks_due_idx ON cleanup_tasks(status, next_retry_at);
CREATE INDEX cleanup_tasks_user_idx ON cleanup_tasks(user_id, status);

CREATE TRIGGER cleanup_tasks_touch_updated_at
BEFORE UPDATE ON cleanup_tasks
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

-- ---------------------------------------------------------------------------
-- Queue and worker coordination
-- ---------------------------------------------------------------------------

CREATE TABLE worker_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  job_type text NOT NULL,
  status text NOT NULL DEFAULT 'queued'
    CHECK (status IN ('queued', 'running', 'retrying', 'completed', 'failed', 'cancelled')),
  idempotency_key text NOT NULL UNIQUE,
  user_id uuid REFERENCES users(id) ON DELETE CASCADE,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  scheduled_for timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  completed_at timestamptz,
  attempt_count integer NOT NULL DEFAULT 0,
  max_attempts integer NOT NULL DEFAULT 3,
  locked_by text,
  locked_at timestamptz,
  next_retry_at timestamptz,
  failure_reason text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX worker_jobs_due_idx ON worker_jobs(status, scheduled_for, next_retry_at);
CREATE INDEX worker_jobs_user_idx ON worker_jobs(user_id, created_at DESC);

CREATE TRIGGER worker_jobs_touch_updated_at
BEFORE UPDATE ON worker_jobs
FOR EACH ROW EXECUTE FUNCTION hotkey_touch_updated_at();

COMMIT;
