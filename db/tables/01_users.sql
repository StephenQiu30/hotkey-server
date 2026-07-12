create table users (
  id bigserial primary key,
  email text not null unique,
  password_hash text not null,
  display_name text not null default '',
  status text not null default 'active',
  plan_type text not null default 'free',
  verification_status text not null default 'unverified',
  email_verified_at timestamptz,
  password_changed_at timestamptz not null default now(),
  last_login_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);


alter table users add column if not exists verification_status text not null default 'unverified';
alter table users add column if not exists email_verified_at timestamptz;
alter table users add column if not exists password_changed_at timestamptz;
alter table users add column if not exists last_login_at timestamptz;
