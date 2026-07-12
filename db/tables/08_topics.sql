create table topics (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  topic_key text not null,
  title text not null,
  summary text not null default '',
  status text not null default 'active',
  first_detected_at timestamptz not null default now(),
  last_active_at timestamptz not null default now(),
  current_heat_score numeric(10,4) not null default 0,
  trend_direction text not null default 'flat',
  representative_post_id bigint references platform_posts(id),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(monitor_id, topic_key)
);



create index idx_topics_monitor_id on topics(monitor_id);
