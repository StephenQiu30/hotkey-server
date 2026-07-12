create table hot_events (
  id bigserial primary key,
  name text not null,
  heat_score double precision not null default 0,
  platform text not null default 'multi',
  trend text not null default 'stable',
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  peak_at timestamptz,
  topic_ids bigint[] default '{}',
  post_ids bigint[] default '{}',
  summary text not null default '',
  category text not null default '',
  status text not null default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);



create index idx_hot_events_heat_score on hot_events(heat_score desc);
create index idx_hot_events_status on hot_events(status);
create index idx_hot_events_platform on hot_events(platform);
create index idx_hot_events_last_seen on hot_events(last_seen_at desc);
