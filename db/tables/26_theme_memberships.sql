create table theme_memberships (
  id bigserial primary key,
  theme_id bigint not null references themes(id),
  event_id bigint references events(id),
  topic_id bigint references topics(id),
  source_kind text not null,
  created_at timestamptz not null default now()
);



create index idx_theme_memberships_theme_id on theme_memberships(theme_id);
create index idx_theme_memberships_event_id on theme_memberships(event_id);
create index idx_theme_memberships_topic_id on theme_memberships(topic_id);
