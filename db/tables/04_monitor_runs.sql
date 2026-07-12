create table monitor_runs (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  platform text not null default 'x',
  run_type text not null default 'poll',
  status text not null default 'pending',
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  fetched_count integer not null default 0,
  stored_count integer not null default 0,
  error_message text not null default '',
  cursor_snapshot jsonb not null default '{}'
);


-- platform content & hits


create index idx_monitor_runs_monitor_id on monitor_runs(monitor_id);
