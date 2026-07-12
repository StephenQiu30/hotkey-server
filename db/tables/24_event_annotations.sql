create table event_annotations (
  id bigserial primary key,
  event_id bigint not null references events(id) unique,
  manual_tags jsonb not null default '[]'::jsonb,
  analyst_conclusion text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

