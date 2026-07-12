create table reports (
  id bigserial primary key,
  user_id bigint not null references users(id),
  report_type text not null check (report_type in ('daily', 'weekly')),
  period_start date not null,
  period_end date not null,
  subject text not null,
  summary text not null default '',
  content text not null default '',
  hotspot_count integer not null default 0,
  status text not null default 'draft' check (status in ('draft', 'sent')),
  sent_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);


-- report exports (Obsidian daily digest)


create index idx_reports_user_type_created on reports(user_id, report_type, created_at desc);
