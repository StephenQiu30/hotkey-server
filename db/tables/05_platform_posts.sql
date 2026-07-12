create table platform_posts (
  id bigserial primary key,
  platform text not null default 'x',
  platform_post_id text not null,
  author_platform_id text not null default '',
  author_name text not null default '',
  author_handle text not null default '',
  content_text text not null default '',
  content_lang text not null default '',
  post_url text not null default '',
  published_at timestamptz,
  like_count integer not null default 0,
  reply_count integer not null default 0,
  repost_count integer not null default 0,
  quote_count integer not null default 0,
  view_count integer not null default 0,
  raw_payload jsonb not null default '{}',
  normalized_hash text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(platform, platform_post_id)
);


alter table platform_posts add column if not exists embedding vector(384);

create index if not exists idx_platform_posts_embedding on platform_posts
