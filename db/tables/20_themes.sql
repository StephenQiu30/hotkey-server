create table themes (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  theme_key text not null,
  title text not null,
  summary text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (monitor_id, theme_key)
);



create index idx_themes_monitor_id on themes(monitor_id);
