-- Migration 001: Add topic_daily_exports table for daily digest publishing.
-- Idempotent: uses IF NOT EXISTS so re-applying is safe.

create table if not exists topic_daily_exports (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  topic_id bigint not null references topics(id),
  export_date date not null,
  summary_text text not null default '',
  markdown_path text not null default '',
  status text not null default 'pending',
  error_message text not null default '',
  published_at timestamptz,
  created_at timestamptz not null default now(),
  unique(monitor_id, topic_id, export_date)
);

create index if not exists idx_topic_daily_exports_monitor_id on topic_daily_exports(monitor_id);
create index if not exists idx_topic_daily_exports_topic_id on topic_daily_exports(topic_id);
