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


-- knowledge writeback audit


create index idx_email_deliveries_notification_id on email_deliveries(notification_id);
