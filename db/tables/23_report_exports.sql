create table report_exports (
  id bigserial primary key,
  report_id bigint not null references reports(id),
  export_kind text not null check (export_kind in ('daily-digest', 'publish-draft')),
  target_path text not null,
  status text not null default 'pending' check (status in ('pending', 'published', 'skipped', 'failed')),
  error_message text not null default '',
  published_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (report_id, export_kind)
);



create index idx_report_exports_report_id on report_exports(report_id);
create index idx_report_exports_status on report_exports(status);
