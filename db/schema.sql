-- hotkey-server PostgreSQL schema
-- Single source of truth for all table definitions (24 tables).

-- users & monitors

create table users (
  id bigserial primary key,
  email text not null unique,
  password_hash text not null,
  display_name text not null default '',
  status text not null default 'active',
  plan_type text not null default 'free',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table keyword_monitors (
  id bigserial primary key,
  user_id bigint not null references users(id),
  name text not null,
  query_text text not null,
  language text not null default 'en',
  region text not null default '',
  status text not null default 'active',
  poll_interval_minutes integer not null default 10,
  alert_enabled boolean not null default false,
  alert_threshold_config jsonb not null default '{}',
  last_polled_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index idx_keyword_monitors_user_id on keyword_monitors(user_id);
create index idx_keyword_monitors_status on keyword_monitors(status);

create table monitor_runs (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  platform text not null default 'x',
  run_type text not null default 'poll',
  status text not null default 'pending',
  started_at timestamptz not null default now(),
  finished_at timestamptz,
  fetched_count integer not null default 0,
  stored_count integer not null default 0,
  error_message text not null default '',
  cursor_snapshot jsonb not null default '{}'
);

create index idx_monitor_runs_monitor_id on monitor_runs(monitor_id);

-- platform content & hits

create table platform_posts (
  id bigserial primary key,
  platform text not null default 'x',
  platform_post_id text not null,
  author_platform_id text not null default '',
  author_name text not null default '',
  author_handle text not null default '',
  content_text text not null default '',
  content_lang text not null default '',
  post_url text not null default '',
  published_at timestamptz,
  like_count integer not null default 0,
  reply_count integer not null default 0,
  repost_count integer not null default 0,
  quote_count integer not null default 0,
  view_count integer not null default 0,
  raw_payload jsonb not null default '{}',
  normalized_hash text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(platform, platform_post_id)
);

create table platform_authors (
  id bigserial primary key,
  platform text not null default 'x',
  platform_author_id text not null,
  handle text not null default '',
  display_name text not null default '',
  followers_count integer not null default 0,
  verified boolean not null default false,
  raw_payload jsonb not null default '{}',
  updated_at timestamptz not null default now(),
  unique(platform, platform_author_id)
);

create table monitor_post_hits (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  post_id bigint not null references platform_posts(id),
  matched_keywords jsonb not null default '[]',
  relevance_score numeric(10,4) not null default 0,
  heat_score numeric(10,4) not null default 0,
  freshness_score numeric(10,4) not null default 0,
  author_influence_score numeric(10,4) not null default 0,
  final_score numeric(10,4) not null default 0,
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  check (jsonb_typeof(matched_keywords) = 'array'),
  unique(monitor_id, post_id)
);

create index idx_monitor_post_hits_monitor_id on monitor_post_hits(monitor_id);
create index idx_monitor_post_hits_post_id on monitor_post_hits(post_id);

-- topics & trends

create table topics (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  topic_key text not null,
  title text not null,
  summary text not null default '',
  status text not null default 'active',
  first_detected_at timestamptz not null default now(),
  last_active_at timestamptz not null default now(),
  current_heat_score numeric(10,4) not null default 0,
  trend_direction text not null default 'flat',
  representative_post_id bigint references platform_posts(id),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique(monitor_id, topic_key)
);

create index idx_topics_monitor_id on topics(monitor_id);

create table topic_posts (
  id bigserial primary key,
  topic_id bigint not null references topics(id),
  post_id bigint not null references platform_posts(id),
  membership_score numeric(10,4) not null default 0,
  is_representative boolean not null default false,
  added_at timestamptz not null default now(),
  unique(topic_id, post_id)
);

create table topic_snapshots (
  id bigserial primary key,
  topic_id bigint not null references topics(id),
  snapshot_time timestamptz not null,
  post_count integer not null default 0,
  unique_author_count integer not null default 0,
  engagement_sum integer not null default 0,
  heat_score numeric(10,4) not null default 0,
  trend_velocity numeric(10,4) not null default 0,
  unique(topic_id, snapshot_time)
);

create index idx_topic_snapshots_topic_id on topic_snapshots(topic_id, snapshot_time);

create table monitor_snapshots (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  snapshot_time timestamptz not null,
  new_post_count integer not null default 0,
  active_topic_count integer not null default 0,
  total_engagement integer not null default 0,
  top_topic_id bigint references topics(id),
  unique(monitor_id, snapshot_time)
);

create index idx_monitor_snapshots_monitor_id on monitor_snapshots(monitor_id, snapshot_time);

-- daily digest exports

create table topic_daily_exports (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  topic_id bigint not null references topics(id),
  export_date date not null,
  summary_text text not null default '',
  markdown_path text not null default '',
  status text not null default 'pending' check (status in ('pending', 'published', 'failed')),
  error_message text not null default '',
  published_at timestamptz,
  created_at timestamptz not null default now(),
  unique(monitor_id, topic_id, export_date)
);

create index idx_topic_daily_exports_monitor_id on topic_daily_exports(monitor_id);
create index idx_topic_daily_exports_topic_id on topic_daily_exports(topic_id);
create index idx_topic_daily_exports_monitor_date on topic_daily_exports(monitor_id, export_date);

-- alerts & notifications

create table alerts (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  topic_id bigint references topics(id),
  alert_type text not null default 'threshold',
  title text not null,
  message text not null default '',
  severity text not null default 'info',
  trigger_score numeric(10,4) not null default 0,
  trigger_reason text not null default '',
  created_at timestamptz not null default now()
);

create index idx_alerts_monitor_id on alerts(monitor_id);

create table user_notifications (
  id bigserial primary key,
  user_id bigint not null references users(id),
  alert_id bigint not null references alerts(id),
  channel text not null default 'in_app',
  delivery_status text not null default 'pending',
  read_at timestamptz,
  sent_at timestamptz,
  created_at timestamptz not null default now()
);

create index idx_user_notifications_user_id on user_notifications(user_id);

create table email_deliveries (
  id bigserial primary key,
  notification_id bigint not null references user_notifications(id),
  recipient_email text not null,
  provider text not null default '',
  provider_message_id text not null default '',
  status text not null default 'pending',
  error_message text not null default '',
  sent_at timestamptz
);

create index idx_email_deliveries_notification_id on email_deliveries(notification_id);

-- knowledge writeback audit

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

create index idx_knowledge_writeback_logs_object on knowledge_writeback_logs(object_type, object_id);
create index idx_knowledge_writeback_logs_status on knowledge_writeback_logs(status);

-- event & knowledge model (STE-356)

create table events (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  event_key text not null,
  title text not null,
  summary text not null default '',
  machine_status text not null default 'active',
  source_post_id bigint references platform_posts(id),
  first_seen_at timestamptz not null,
  last_active_at timestamptz not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (monitor_id, event_key)
);

create index idx_events_monitor_id on events(monitor_id);

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

create table knowledge_runs (
  id bigserial primary key,
  run_key text not null unique,
  run_type text not null,
  target_date date,
  status text not null default 'pending',
  error_message text not null default '',
  started_at timestamptz,
  finished_at timestamptz,
  created_at timestamptz not null default now()
);

create table themes (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  theme_key text not null,
  title text not null,
  summary text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (monitor_id, theme_key)
);

create index idx_themes_monitor_id on themes(monitor_id);

create table export_bundles (
  id bigserial primary key,
  monitor_id bigint not null references keyword_monitors(id),
  bundle_key text not null,
  bundle_kind text not null,
  date_start date,
  date_end date,
  status text not null default 'pending',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (monitor_id, bundle_key)
);

create index idx_export_bundles_monitor_id on export_bundles(monitor_id);

create table reports (
  id bigserial primary key,
  user_id bigint not null references users(id),
  report_type text not null check (report_type in ('daily', 'weekly')),
  period_start date not null,
  period_end date not null,
  subject text not null,
  summary text not null default '',
  content text not null default '',
  hotspot_count integer not null default 0,
  status text not null default 'draft' check (status in ('draft', 'sent')),
  sent_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index idx_reports_user_type_created on reports(user_id, report_type, created_at desc);

-- report exports (Obsidian daily digest)

create table report_exports (
  id bigserial primary key,
  report_id bigint not null references reports(id),
  export_kind text not null check (export_kind in ('daily-digest', 'publish-draft')),
  target_path text not null,
  status text not null default 'pending' check (status in ('pending', 'published', 'skipped', 'failed')),
  error_message text not null default '',
  published_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (report_id, export_kind)
);

create index idx_report_exports_report_id on report_exports(report_id);
create index idx_report_exports_status on report_exports(status);

create table event_annotations (
  id bigserial primary key,
  event_id bigint not null references events(id) unique,
  manual_tags jsonb not null default '[]'::jsonb,
  analyst_conclusion text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table topic_annotations (
  id bigserial primary key,
  topic_id bigint not null references topics(id) unique,
  material_status text not null default 'unreviewed',
  manual_summary text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

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

create table knowledge_object_revisions (
  id bigserial primary key,
  object_type text not null,
  object_id bigint not null,
  revision text not null,
  source_path text not null default '',
  updated_at timestamptz not null default now(),
  unique (object_type, object_id)
);

-- hot event model for multi-platform hot events

create table hot_events (
  id bigserial primary key,
  name text not null,
  heat_score double precision not null default 0,
  platform text not null default 'multi',
  trend text not null default 'stable',
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  peak_at timestamptz,
  topic_ids bigint[] default '{}',
  post_ids bigint[] default '{}',
  summary text not null default '',
  category text not null default '',
  status text not null default 'active',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create index idx_hot_events_heat_score on hot_events(heat_score desc);
create index idx_hot_events_status on hot_events(status);
create index idx_hot_events_platform on hot_events(platform);
create index idx_hot_events_last_seen on hot_events(last_seen_at desc);

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

create index idx_hot_event_platforms_event_id on hot_event_platforms(hot_event_id);
