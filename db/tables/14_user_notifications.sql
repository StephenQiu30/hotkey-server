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
