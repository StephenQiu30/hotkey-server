create table dead_letter_records (
  id              bigserial primary key,
  topic           varchar(255) not null,
  message_id      varchar(128) not null,
  message_type    varchar(64)  not null,
  payload         text,
  error_message   text,
  retry_count     int          not null default 0,
  created_at      timestamptz  not null default now()
);


-- pgvector extension for cosine similarity matching

-- platform_posts: embedding vector for semantic matching
  using ivfflat (embedding vector_cosine_ops) with (lists = 100);

-- keyword_monitors: embedding vector for query text

-- users: auth columns added after initial migration


create index idx_dead_letter_created_at on dead_letter_records(created_at);
