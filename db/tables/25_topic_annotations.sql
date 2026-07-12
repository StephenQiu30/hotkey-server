create table topic_annotations (
  id bigserial primary key,
  topic_id bigint not null references topics(id) unique,
  material_status text not null default 'unreviewed',
  manual_summary text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

