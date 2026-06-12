-- Base schema for hotkey-server
-- Plan 002 foundation tables + Plan 003 ingestion tables

-- Users (Plan 002)
create table users (
  id bigserial primary key,
  email text not null unique,
  password_hash text not null,
  display_name text not null,
  status text not null default 'active',
  plan_type text not null default 'free',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- Keyword monitors (Plan 002)
create table keyword_monitors (
  id bigserial primary key,
  user_id bigint not null references users(id),
  name text not null,
  query_text text not null,
  language text not null default 'en',
  region text not null default 'global',
  status text not null default 'active',
  poll_interval_minutes integer not null,
  alert_enabled boolean not null default true,
  alert_threshold_config jsonb not null default '{}'::jsonb,
  last_polled_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

-- Monitor runs (Plan 003)
create table monitor_runs (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  platform text not null,
  run_type text not null,
  status text not null,
  started_at timestamptz not null,
  finished_at timestamptz,
  fetched_count integer not null default 0,
  stored_count integer not null default 0,
  error_message text,
  cursor_snapshot jsonb not null default '{}'::jsonb
);

-- Platform authors (Plan 003)
create table platform_authors (
  id bigserial primary key,
  platform text not null,
  platform_author_id text not null,
  handle text not null,
  display_name text not null,
  followers_count integer not null default 0,
  verified boolean not null default false,
  raw_payload jsonb not null default '{}'::jsonb,
  updated_at timestamptz not null default now(),
  unique(platform, platform_author_id)
);

-- Platform posts (Plan 003)
create table platform_posts (
  id bigserial primary key,
  platform text not null,
  platform_post_id text not null,
  author_platform_id text not null,
  author_name text not null,
  author_handle text not null,
  content_text text not null,
  content_lang text not null,
  post_url text not null,
  published_at timestamptz not null,
  like_count integer not null default 0,
  reply_count integer not null default 0,
  repost_count integer not null default 0,
  quote_count integer not null default 0,
  view_count integer not null default 0,
  raw_payload jsonb not null default '{}'::jsonb,
  normalized_hash text not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(platform, platform_post_id)
);

-- Monitor post hits (Plan 003)
create table monitor_post_hits (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  post_id bigint not null references platform_posts(id),
  matched_keywords jsonb not null default '[]'::jsonb,
  relevance_score numeric(5,4) not null default 0,
  heat_score numeric(10,4) not null default 0,
  freshness_score numeric(10,4) not null default 0,
  author_influence_score numeric(10,4) not null default 0,
  final_score numeric(10,4) not null default 0,
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  unique(monitor_id, post_id)
);
