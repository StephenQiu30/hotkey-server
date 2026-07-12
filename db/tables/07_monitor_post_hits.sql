create table monitor_post_hits (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  post_id bigint not null references platform_posts(id),
  matched_keywords jsonb not null default '[]',
  relevance_score numeric(10,4) not null default 0,
  heat_score numeric(10,4) not null default 0,
  freshness_score numeric(10,4) not null default 0,
  author_influence_score numeric(10,4) not null default 0,
  final_score numeric(10,4) not null default 0,
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  check (jsonb_typeof(matched_keywords) = 'array'),
  unique(monitor_id, post_id)
);


-- topics & trends


create index idx_monitor_post_hits_monitor_id on monitor_post_hits(monitor_id);
create index idx_monitor_post_hits_post_id on monitor_post_hits(post_id);
