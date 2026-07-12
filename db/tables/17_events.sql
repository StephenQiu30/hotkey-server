create table events (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  event_key text not null,
  title text not null,
  summary text not null default '',
  machine_status text not null default 'active',
  source_post_id bigint references platform_posts(id),
  first_seen_at timestamptz not null,
  last_active_at timestamptz not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (monitor_id, event_key)
);



create index idx_events_monitor_id on events(monitor_id);
