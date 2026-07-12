create table auth_sessions (
  id bigserial primary key,
  user_id bigint not null references users(id) on delete cascade,
  token_hash text not null unique,
  family_hash text not null,
  status text not null default 'active',
  ip_address text not null default '',
  user_agent text not null default '',
  expires_at timestamptz not null,
  absolute_expires_at timestamptz not null,
  last_refreshed_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);



create index idx_auth_sessions_user_id on auth_sessions(user_id);
create index idx_auth_sessions_family_hash on auth_sessions(family_hash);
create index idx_auth_sessions_expires_at on auth_sessions(expires_at);
