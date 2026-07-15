#!/usr/bin/env sh
set -eu

dsn=${HOTKEY_TEST_DSN:-}
if test -z "$dsn"; then
  printf '%s\n' 'HOTKEY_TEST_DSN is required' >&2
  exit 1
fi

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
for run in 1 2; do
  psql "$dsn" -v ON_ERROR_STOP=1 -f "$root/db/schema.sql" >/dev/null
done

psql "$dsn" -v ON_ERROR_STOP=1 <<'SQL' >/dev/null
DO $$
BEGIN
  BEGIN
    INSERT INTO monitors (name, relevance_threshold) VALUES ('schema-invalid-score', -1);
    RAISE EXCEPTION 'missing monitor score constraint';
  EXCEPTION WHEN check_violation THEN
    NULL;
  END;

  BEGIN
    INSERT INTO source_connections (source_type, name, endpoint) VALUES ('invalid-source', 'invalid-source', 'https://example.test');
    RAISE EXCEPTION 'missing source status constraint';
  EXCEPTION WHEN check_violation THEN
    NULL;
  END;

  BEGIN
    INSERT INTO monitor_sources (monitor_id, source_connection_id) VALUES (999999, 999999);
    RAISE EXCEPTION 'missing monitor source foreign keys';
  EXCEPTION WHEN foreign_key_violation THEN
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

  BEGIN
    INSERT INTO source_connections (source_type, name, endpoint) VALUES ('rss', 'schema-idempotency-source', 'https://example.test/rss');
    INSERT INTO monitors (name) VALUES ('schema-idempotency-monitor');
    INSERT INTO monitor_sources (monitor_id, source_connection_id)
      VALUES ((SELECT id FROM monitors WHERE name = 'schema-idempotency-monitor'), (SELECT id FROM source_connections WHERE name = 'schema-idempotency-source'));
    INSERT INTO collection_runs (monitor_source_id, idempotency_key, trigger_type, scheduled_at)
      VALUES ((SELECT id FROM monitor_sources ORDER BY id DESC LIMIT 1), 'schema-idempotency-key', 'manual', now());
    INSERT INTO collection_runs (monitor_source_id, idempotency_key, trigger_type, scheduled_at)
      VALUES ((SELECT id FROM monitor_sources ORDER BY id DESC LIMIT 1), 'schema-idempotency-key', 'manual', now());
    RAISE EXCEPTION 'missing collection run idempotency constraint';
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
if test "$application_tables" -ne 49; then
  printf 'application table count = %s, want 49\n' "$application_tables" >&2
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
