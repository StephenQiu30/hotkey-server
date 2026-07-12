create table keyword_monitors (
  id bigserial primary key,
  user_id bigint not null references users(id),
  name text not null,
  query_text text not null,
  language text not null default 'en',
  region text not null default '',
  status text not null default 'active',
  poll_interval_minutes integer not null default 10,
  alert_enabled boolean not null default false,
  alert_threshold_config jsonb not null default '{}',
  last_polled_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);



alter table keyword_monitors add column if not exists query_embedding vector(384);

create index idx_keyword_monitors_user_id on keyword_monitors(user_id);
create index idx_keyword_monitors_status on keyword_monitors(status);
