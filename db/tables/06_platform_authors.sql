create table platform_authors (
  id bigserial primary key,
  platform text not null default 'x',
  platform_author_id text not null,
  handle text not null default '',
  display_name text not null default '',
  followers_count integer not null default 0,
  verified boolean not null default false,
  raw_payload jsonb not null default '{}',
  updated_at timestamptz not null default now(),
  unique(platform, platform_author_id)
);

