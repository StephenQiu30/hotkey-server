create table topic_posts (
  id bigserial primary key,
  topic_id bigint not null references topics(id),
  post_id bigint not null references platform_posts(id),
  membership_score numeric(10,4) not null default 0,
  is_representative boolean not null default false,
  added_at timestamptz not null default now(),
  unique(topic_id, post_id)
);

