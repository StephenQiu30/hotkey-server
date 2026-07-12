create table alerts (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  topic_id bigint references topics(id),
  alert_type text not null default 'threshold',
  title text not null,
  message text not null default '',
  severity text not null default 'info',
  trigger_score numeric(10,4) not null default 0,
  trigger_reason text not null default '',
  created_at timestamptz not null default now()
);



create index idx_alerts_monitor_id on alerts(monitor_id);
