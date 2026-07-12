create table knowledge_object_revisions (
  id bigserial primary key,
  object_type text not null,
  object_id bigint not null,
  revision text not null,
  source_path text not null default '',
  updated_at timestamptz not null default now(),
  unique (object_type, object_id)
);

-- hot event model for multi-platform hot events

