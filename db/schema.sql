-- HotKey Server - Core Database Schema
-- This schema defines the foundational tables for users and keyword monitors.

create table if not exists users (
  id bigserial primary key,
  email text not null unique,
  password_hash text not null,
  display_name text not null,
  status text not null default 'active',
  plan_type text not null default 'free',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table if not exists keyword_monitors (
  id bigserial primary key,
  user_id bigint not null references users(id),
  name text not null,
  query_text text not null,
  language text not null default 'en',
  region text not null default 'global',
  status text not null default 'active',
  poll_interval_minutes integer not null,
  alert_enabled boolean not null default true,
  alert_threshold_config jsonb not null default '{}'::jsonb,
  last_polled_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
