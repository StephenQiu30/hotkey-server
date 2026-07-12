create table monitor_snapshots (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  snapshot_time timestamptz not null,
  new_post_count integer not null default 0,
  active_topic_count integer not null default 0,
  total_engagement integer not null default 0,
  top_topic_id bigint references topics(id),
  unique(monitor_id, snapshot_time)
);


-- daily digest exports


create index idx_monitor_snapshots_monitor_id on monitor_snapshots(monitor_id, snapshot_time);
