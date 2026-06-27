do $$
begin
  if not exists (
    select 1
    from pg_constraint
    where conname = 'topic_snapshots_topic_id_snapshot_time_key'
      and conrelid = 'topic_snapshots'::regclass
  ) then
    alter table topic_snapshots
      add constraint topic_snapshots_topic_id_snapshot_time_key
      unique (topic_id, snapshot_time);
  end if;

  if not exists (
    select 1
    from pg_constraint
    where conname = 'monitor_snapshots_monitor_id_snapshot_time_key'
      and conrelid = 'monitor_snapshots'::regclass
  ) then
    alter table monitor_snapshots
      add constraint monitor_snapshots_monitor_id_snapshot_time_key
      unique (monitor_id, snapshot_time);
  end if;
end $$;
