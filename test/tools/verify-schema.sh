#!/usr/bin/env sh
set -eu

dsn=${HOTKEY_TEST_DSN:-}
if test -z "$dsn"; then
  printf '%s\n' 'HOTKEY_TEST_DSN is required' >&2
  exit 1
fi

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
for run in 1 2; do
  psql "$dsn" -v ON_ERROR_STOP=1 -f "$root/db/schema.sql" >/dev/null
done

psql "$dsn" -v ON_ERROR_STOP=1 <<'SQL' >/dev/null
DO $$
DECLARE
  suffix text := md5(clock_timestamp()::text || random()::text);
  source_id bigint;
  first_monitor_id bigint;
  second_monitor_id bigint;
  first_config_id bigint;
  second_config_id bigint;
  monitor_source_id bigint;
  content_id bigint;
  collection_run_id bigint;
  collection_run_target_id bigint;
  collection_run_item_id bigint;
BEGIN
  INSERT INTO source_connections (source_type, name, endpoint)
    VALUES ('rss', 'schema-source-' || suffix, 'https://example.test/schema')
    RETURNING id INTO source_id;
  INSERT INTO monitors (name) VALUES ('schema-monitor-first-' || suffix) RETURNING id INTO first_monitor_id;
  INSERT INTO monitors (name) VALUES ('schema-monitor-second-' || suffix) RETURNING id INTO second_monitor_id;
  INSERT INTO monitor_config_versions (monitor_id, revision) VALUES (first_monitor_id, 1) RETURNING id INTO first_config_id;
  INSERT INTO monitor_rules (config_version_id, rule_type, operator, value, weight, approval_status)
    VALUES (first_config_id, 'keyword', 'contains', 'schema', 10, 'approved');
  INSERT INTO monitor_sources (config_version_id, source_connection_id)
    VALUES (first_config_id, source_id) RETURNING id INTO monitor_source_id;
  UPDATE monitor_config_versions
    SET state = 'published', config_hash = repeat('a', 64), published_at = now()
    WHERE id = first_config_id;

  BEGIN
    UPDATE monitor_rules SET value = 'mutated' WHERE config_version_id = first_config_id;
    RAISE EXCEPTION 'missing published monitor rule immutability trigger';
  EXCEPTION WHEN check_violation THEN
    NULL;
  END;

  BEGIN
    UPDATE monitor_sources SET priority = 1 WHERE id = monitor_source_id;
    RAISE EXCEPTION 'missing published monitor source immutability trigger';
  EXCEPTION WHEN check_violation THEN
    NULL;
  END;

  BEGIN
    UPDATE source_connections SET endpoint = 'https://example.test/changed' WHERE id = source_id;
    RAISE EXCEPTION 'missing published source semantic immutability trigger';
  EXCEPTION WHEN check_violation THEN
    NULL;
  END;

  INSERT INTO monitor_config_versions (monitor_id, revision) VALUES (second_monitor_id, 1) RETURNING id INTO second_config_id;
  BEGIN
    UPDATE monitors SET published_config_version_id = first_config_id WHERE id = second_monitor_id;
    RAISE EXCEPTION 'missing monitor config ownership trigger';
  EXCEPTION WHEN check_violation THEN
    NULL;
  END;

  INSERT INTO contents (source_connection_id, external_id, content_type, canonical_url, published_at, fetched_at, dedupe_key)
    VALUES (source_id, 'schema-content-' || suffix, 'article', 'https://example.test/content', now(), now(), repeat('b', 64))
    RETURNING id INTO content_id;
  BEGIN
    INSERT INTO monitor_matches (
      monitor_id, monitor_config_version_id, content_id, rule_score, final_score, decision, algorithm_version,
      input_hash, scoring_version
    ) VALUES (
      second_monitor_id, first_config_id, content_id, 10, 10, 'accepted', 'schema', repeat('c', 64), 'schema-v1'
    );
    RAISE EXCEPTION 'missing monitor/config composite foreign key';
  EXCEPTION WHEN foreign_key_violation THEN
    NULL;
  END;

  INSERT INTO collection_runs (source_connection_id, query_signature, window_start, window_end, trigger_type, scheduled_at)
    VALUES (source_id, repeat('c', 64), now(), now() + interval '1 minute', 'manual', now())
    RETURNING id INTO collection_run_id;
  BEGIN
    INSERT INTO collection_runs (source_connection_id, query_signature, window_start, window_end, trigger_type, scheduled_at)
      VALUES (source_id, repeat('c', 64), now(), now() + interval '1 minute', 'manual', now());
    RAISE EXCEPTION 'missing shared collection run four-tuple uniqueness';
  EXCEPTION WHEN unique_violation THEN
    NULL;
  END;
  BEGIN
    INSERT INTO collection_run_targets (collection_run_id, monitor_source_id, monitor_config_version_id)
      VALUES (collection_run_id, monitor_source_id, second_config_id);
    RAISE EXCEPTION 'missing monitor source/config composite foreign key';
  EXCEPTION WHEN foreign_key_violation THEN
    NULL;
  END;

  INSERT INTO collection_run_targets (collection_run_id, monitor_source_id, monitor_config_version_id)
    VALUES (collection_run_id, monitor_source_id, first_config_id)
    RETURNING id INTO collection_run_target_id;
  INSERT INTO collection_run_items (
      run_id, source_connection_id, source_code, external_id, content_type, captured_item_version, captured_item,
      payload_hash, raw_payload_disposition, outcome, observed_at
  ) VALUES (
      collection_run_id, source_id, 'rss', 'schema-item-' || suffix, 'article', 'v1', '{"title":"safe"}'::jsonb,
      repeat('d', 64), 'discarded', 'captured', now()
  ) RETURNING id INTO collection_run_item_id;
  INSERT INTO collection_run_target_items (collection_run_id, collection_run_target_id, collection_run_item_id, outcome)
    VALUES (collection_run_id, collection_run_target_id, collection_run_item_id, 'captured');
  INSERT INTO source_checkpoints (monitor_source_id, query_hash, last_successful_run_id, last_fetched_at, next_poll_at)
    VALUES (monitor_source_id, repeat('e', 64), collection_run_id, now(), now() + interval '5 minutes');
  BEGIN
    INSERT INTO collection_run_target_items (collection_run_id, collection_run_target_id, collection_run_item_id, outcome)
      VALUES (collection_run_id, collection_run_target_id, collection_run_item_id, 'captured');
    RAISE EXCEPTION 'missing collection target-item reconciliation uniqueness';
  EXCEPTION WHEN unique_violation THEN
    NULL;
  END;
  BEGIN
    INSERT INTO collection_run_items (
        run_id, source_connection_id, source_code, external_id, content_type, captured_item_version, captured_item,
        payload_hash, raw_payload_disposition, outcome, observed_at
    ) VALUES (
        collection_run_id, source_id, 'rss', 'schema-invalid-item-' || suffix, 'article', 'v1', '{"title":"safe"}'::jsonb,
        repeat('f', 64), 'raw_response', 'captured', now()
    );
    RAISE EXCEPTION 'missing raw payload disposition check';
  EXCEPTION WHEN check_violation THEN
    NULL;
  END;

  BEGIN
    INSERT INTO knowledge_documents (document_type, vault_path) VALUES ('event', 'invalid/no-target.md');
    RAISE EXCEPTION 'missing exactly-one knowledge target constraint';
  EXCEPTION WHEN check_violation THEN
    NULL;
  END;

  BEGIN
    INSERT INTO users (email, password_hash, display_name, role) VALUES ('schema-duplicate@example.test', 'x', 'schema', 'viewer');
    INSERT INTO users (email, password_hash, display_name, role) VALUES ('SCHEMA-DUPLICATE@example.test', 'x', 'schema', 'viewer');
    RAISE EXCEPTION 'missing active email uniqueness constraint';
  EXCEPTION WHEN unique_violation THEN
    NULL;
  END;

  DECLARE
    identity_user_id bigint;
    identity_session_id bigint;
    identity_session_expires_at timestamptz;
    identity_family_id uuid := md5(clock_timestamp()::text || random()::text)::uuid;
    identity_token_hash text := repeat(md5(clock_timestamp()::text || random()::text), 2);
  BEGIN
    INSERT INTO users (email, password_hash, display_name, role)
      VALUES ('schema-auth-constraint@example.test', 'x', 'schema auth', 'viewer')
      ON CONFLICT DO NOTHING;
    SELECT id INTO identity_user_id FROM users WHERE email = 'schema-auth-constraint@example.test' AND deleted_at IS NULL;
    INSERT INTO auth_sessions (user_id, family_id, absolute_expires_at)
      VALUES (identity_user_id, identity_family_id, now() + interval '30 days')
      RETURNING id, absolute_expires_at INTO identity_session_id, identity_session_expires_at;

    BEGIN
      INSERT INTO auth_sessions (user_id, family_id, absolute_expires_at)
        VALUES (identity_user_id, identity_family_id, now() + interval '30 days');
      RAISE EXCEPTION 'missing auth session family uniqueness constraint';
    EXCEPTION WHEN unique_violation THEN
      NULL;
    END;

    BEGIN
      INSERT INTO auth_sessions (user_id, family_id, absolute_expires_at, created_at)
        VALUES (identity_user_id, md5(clock_timestamp()::text || random()::text)::uuid, now(), now() + interval '1 second');
      RAISE EXCEPTION 'missing auth session absolute expiry constraint';
    EXCEPTION WHEN check_violation THEN
      NULL;
    END;

    INSERT INTO auth_refresh_tokens (session_id, token_hash, expires_at)
      VALUES (identity_session_id, identity_token_hash, now() + interval '7 days');

    BEGIN
      UPDATE auth_sessions
      SET absolute_expires_at = identity_session_expires_at + interval '1 day'
      WHERE id = identity_session_id;
      RAISE EXCEPTION 'auth session absolute expiry must be immutable';
    EXCEPTION WHEN check_violation THEN
      NULL;
    END;

    IF (SELECT absolute_expires_at FROM auth_sessions WHERE id = identity_session_id) IS DISTINCT FROM identity_session_expires_at THEN
      RAISE EXCEPTION 'auth session absolute expiry changed after rejected update';
    END IF;
    IF (SELECT expires_at FROM auth_refresh_tokens WHERE session_id = identity_session_id AND token_hash = identity_token_hash) IS DISTINCT FROM now() + interval '7 days' THEN
      RAISE EXCEPTION 'auth refresh token changed after rejected session update';
    END IF;

    BEGIN
      INSERT INTO auth_refresh_tokens (session_id, token_hash, expires_at)
        VALUES (identity_session_id, identity_token_hash, now() + interval '6 days');
      RAISE EXCEPTION 'missing auth refresh token hash uniqueness constraint';
    EXCEPTION WHEN unique_violation THEN
      NULL;
    END;

    BEGIN
      INSERT INTO auth_refresh_tokens (session_id, token_hash, expires_at)
        VALUES (identity_session_id + 1000000, repeat('b', 64), now() + interval '7 days');
      RAISE EXCEPTION 'missing auth refresh token session foreign key';
    EXCEPTION WHEN foreign_key_violation THEN
      NULL;
    END;

    BEGIN
      INSERT INTO auth_refresh_tokens (session_id, token_hash, expires_at)
        VALUES (identity_session_id, repeat('c', 64), identity_session_expires_at + interval '1 second');
      RAISE EXCEPTION 'missing auth refresh token parent session expiry constraint';
    EXCEPTION WHEN check_violation THEN
      NULL;
    END;

    IF to_regclass('public.auth_sessions_active_user_idx') IS NULL THEN
      RAISE EXCEPTION 'missing auth session access-path index';
    END IF;
    IF to_regclass('public.auth_refresh_tokens_session_expiry_idx') IS NULL THEN
      RAISE EXCEPTION 'missing auth refresh token access-path index';
    END IF;
  END;
END
$$;
SQL

application_tables=$(psql "$dsn" -Atqc "SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename NOT LIKE 'river_%'")
if test "$application_tables" -ne 58; then
  printf 'application table count = %s, want 58\n' "$application_tables" >&2
  exit 1
fi

river_tables=$(psql "$dsn" -Atqc "SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename LIKE 'river_%'")
if test "$river_tables" -ne 5; then
  printf 'River table count = %s, want 5\n' "$river_tables" >&2
  exit 1
fi

river_names=$(psql "$dsn" -Atqc "SELECT string_agg(tablename, ',' ORDER BY tablename) FROM pg_tables WHERE schemaname = 'public' AND tablename LIKE 'river_%'")
if test "$river_names" != 'river_job,river_job_attempt,river_leader,river_migration,river_queue'; then
  printf 'River table set = %s, want river_job,river_job_attempt,river_leader,river_migration,river_queue\n' "$river_names" >&2
  exit 1
fi

printf '%s\n' 'Schema verification passed.'
