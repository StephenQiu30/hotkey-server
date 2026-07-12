create table export_bundles (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  bundle_key text not null,
  bundle_kind text not null,
  date_start date,
  date_end date,
  status text not null default 'pending',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (monitor_id, bundle_key)
);



create index idx_export_bundles_monitor_id on export_bundles(monitor_id);
