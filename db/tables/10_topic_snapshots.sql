create table topic_snapshots (
  id bigserial primary key,
  topic_id bigint not null references topics(id),
  snapshot_time timestamptz not null,
  post_count integer not null default 0,
  unique_author_count integer not null default 0,
  engagement_sum integer not null default 0,
  heat_score numeric(10,4) not null default 0,
  trend_velocity numeric(10,4) not null default 0,
  unique(topic_id, snapshot_time)
);



create index idx_topic_snapshots_topic_id on topic_snapshots(topic_id, snapshot_time);
