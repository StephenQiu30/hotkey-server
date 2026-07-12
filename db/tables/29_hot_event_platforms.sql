create table hot_event_platforms (
  hot_event_id bigint not null references hot_events(id) on delete cascade,
  platform text not null,
  rank int not null default 0,
  title text not null default '',
  url text not null default '',
  heat double precision not null default 0,
  updated_at timestamptz not null default now(),
  primary key (hot_event_id, platform)
);


-- dead letter queue records for task infrastructure


create index idx_hot_event_platforms_event_id on hot_event_platforms(hot_event_id);
