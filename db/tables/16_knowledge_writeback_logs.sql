create table knowledge_writeback_logs (
  id bigserial primary key,
  object_type text not null,
  object_id bigint not null,
  field_name text not null,
  field_value jsonb not null,
  status text not null,
  conflict_reason text not null default '',
  source_path text not null default '',
  created_at timestamptz not null default now()
);


-- event & knowledge model (STE-356)


create index idx_knowledge_writeback_logs_object on knowledge_writeback_logs(object_type, object_id);
create index idx_knowledge_writeback_logs_status on knowledge_writeback_logs(status);
