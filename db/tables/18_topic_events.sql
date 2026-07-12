create table topic_events (
  id bigserial primary key,
  topic_id bigint not null references topics(id),
  event_id bigint not null references events(id),
  relationship_type text not null default 'member',
  created_at timestamptz not null default now(),
  unique (topic_id, event_id)
);



create index idx_topic_events_topic_id on topic_events(topic_id);
create index idx_topic_events_event_id on topic_events(event_id);
