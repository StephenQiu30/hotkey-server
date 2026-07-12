create table knowledge_runs (
  id bigserial primary key,
  run_key text not null unique,
  run_type text not null,
  target_date date,
  status text not null default 'pending',
  error_message text not null default '',
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now()
);

