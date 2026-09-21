-- HotKey PostgreSQL schema
--
-- This is the only database DDL source of truth. Apply it only to a new, empty
-- database. Business tables, constraints, indexes, functions, and comments are
-- added here together with their SQLAlchemy runtime mappings.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE TABLE identity_users (
    id UUID PRIMARY KEY,
    singleton_key SMALLINT NOT NULL UNIQUE DEFAULT 1 CHECK (singleton_key = 1),
    username VARCHAR(64) NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    credential_version INTEGER NOT NULL DEFAULT 1 CHECK (credential_version >= 1),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE identity_sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    token_digest BYTEA NOT NULL UNIQUE CHECK (octet_length(token_digest) = 32),
    csrf_digest BYTEA NOT NULL CHECK (octet_length(csrf_digest) = 32),
    credential_version INTEGER NOT NULL CHECK (credential_version >= 1),
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL CHECK (expires_at > created_at),
    revoked_at TIMESTAMPTZ,
    revoked_reason VARCHAR(32),
    CHECK (
        (revoked_at IS NULL AND revoked_reason IS NULL)
        OR (revoked_at IS NOT NULL AND revoked_reason IS NOT NULL)
    )
);

CREATE INDEX identity_sessions_user_id_idx ON identity_sessions (user_id);
CREATE INDEX identity_sessions_active_expiry_idx
    ON identity_sessions (expires_at)
    WHERE revoked_at IS NULL;

CREATE TABLE jobs (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    operation_id UUID NOT NULL,
    kind VARCHAR(64) NOT NULL CHECK (kind ~ '^[a-z][a-z0-9_.-]{0,63}$'),
    scope JSONB NOT NULL CHECK (jsonb_typeof(scope) = 'object'),
    request_fingerprint BYTEA NOT NULL CHECK (octet_length(request_fingerprint) = 32),
    status VARCHAR(32) NOT NULL DEFAULT 'queued' CHECK (
        status IN (
            'queued',
            'running',
            'succeeded',
            'partially_succeeded',
            'failed',
            'cancelled'
        )
    ),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
    CONSTRAINT jobs_owner_kind_operation_key UNIQUE (owner_id, kind, operation_id)
);

CREATE TABLE outbox_messages (
    id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    topic VARCHAR(128) NOT NULL,
    message_key UUID NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    created_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,
    CONSTRAINT outbox_messages_event_aggregate_key UNIQUE (event_type, aggregate_id),
    CHECK (published_at IS NULL OR published_at >= created_at)
);

COMMIT;
