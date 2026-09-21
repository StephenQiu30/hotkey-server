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

CREATE TABLE source_access_policies (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    source_key VARCHAR(64) NOT NULL CHECK (source_key ~ '^[a-z][a-z0-9_-]{0,63}$'),
    capability VARCHAR(32) NOT NULL CHECK (
        capability IN ('search', 'author_posts', 'comments', 'replies')
    ),
    status VARCHAR(16) NOT NULL CHECK (status IN ('pending', 'approved', 'blocked')),
    enabled BOOLEAN NOT NULL DEFAULT false,
    access_basis VARCHAR(32) CHECK (
        access_basis IS NULL
        OR access_basis IN (
            'official_api',
            'authorized_feed',
            'written_permission',
            'manual_import'
        )
    ),
    terms_reference VARCHAR(512),
    processing_purpose VARCHAR(256) NOT NULL,
    component_name VARCHAR(128),
    component_version VARCHAR(64),
    component_license VARCHAR(128),
    field_purposes JSONB NOT NULL DEFAULT '{}'::jsonb
        CHECK (jsonb_typeof(field_purposes) = 'object'),
    reviewed_at TIMESTAMPTZ,
    review_expires_at TIMESTAMPTZ,
    policy_version INTEGER NOT NULL DEFAULT 1 CHECK (policy_version >= 1),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
    CONSTRAINT source_access_policies_owner_source_capability_key
        UNIQUE (owner_id, source_key, capability),
    CONSTRAINT source_access_policies_owner_id_id_key UNIQUE (owner_id, id),
    CHECK (NOT enabled OR status = 'approved'),
    CHECK (
        status <> 'approved'
        OR (
            access_basis IS NOT NULL
            AND terms_reference IS NOT NULL
            AND component_name IS NOT NULL
            AND component_version IS NOT NULL
            AND component_license IS NOT NULL
            AND reviewed_at IS NOT NULL
            AND field_purposes <> '{}'::jsonb
        )
    ),
    CHECK (
        review_expires_at IS NULL
        OR (reviewed_at IS NOT NULL AND review_expires_at > reviewed_at)
    )
);

CREATE TABLE evidence_retention_policies (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    source_policy_id UUID NOT NULL,
    source_policy_version INTEGER NOT NULL CHECK (source_policy_version >= 1),
    data_class VARCHAR(16) NOT NULL CHECK (data_class IN ('structured', 'raw', 'media')),
    requested_days INTEGER NOT NULL CHECK (requested_days BETWEEN 0 AND 3650),
    source_max_days INTEGER CHECK (source_max_days BETWEEN 0 AND 3650),
    effective_days INTEGER NOT NULL,
    policy_version INTEGER NOT NULL DEFAULT 1 CHECK (policy_version >= 1),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
    CONSTRAINT evidence_retention_policies_owner_source_class_key
        UNIQUE (owner_id, source_policy_id, data_class),
    CONSTRAINT evidence_retention_policies_owner_id_id_key UNIQUE (owner_id, id),
    CONSTRAINT evidence_retention_policies_owner_source_policy_fkey
        FOREIGN KEY (owner_id, source_policy_id)
        REFERENCES source_access_policies (owner_id, id)
        ON DELETE CASCADE,
    CHECK (
        effective_days = CASE
            WHEN source_max_days IS NULL THEN requested_days
            ELSE LEAST(requested_days, source_max_days)
        END
    )
);

CREATE TABLE evidence_resources (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    resource_type VARCHAR(64) NOT NULL CHECK (
        resource_type ~ '^[a-z][a-z0-9_]{0,63}$'
    ),
    resource_id UUID NOT NULL,
    source_policy_id UUID NOT NULL,
    source_policy_version INTEGER NOT NULL CHECK (source_policy_version >= 1),
    retention_policy_id UUID NOT NULL,
    retention_policy_version INTEGER NOT NULL CHECK (retention_policy_version >= 1),
    data_class VARCHAR(16) NOT NULL CHECK (data_class IN ('structured', 'raw', 'media')),
    collected_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL CHECK (expires_at >= collected_at),
    cleanup_targets JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (
        jsonb_typeof(cleanup_targets) = 'array'
    ),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT evidence_resources_owner_type_resource_key
        UNIQUE (owner_id, resource_type, resource_id),
    CONSTRAINT evidence_resources_owner_id_id_key UNIQUE (owner_id, id),
    CONSTRAINT evidence_resources_owner_source_policy_fkey
        FOREIGN KEY (owner_id, source_policy_id)
        REFERENCES source_access_policies (owner_id, id)
        ON DELETE RESTRICT,
    CONSTRAINT evidence_resources_owner_retention_policy_fkey
        FOREIGN KEY (owner_id, retention_policy_id)
        REFERENCES evidence_retention_policies (owner_id, id)
        ON DELETE RESTRICT
);

CREATE INDEX evidence_resources_expiry_idx ON evidence_resources (expires_at);

CREATE TABLE evidence_deletions (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    operation_id UUID NOT NULL,
    resource_record_id UUID NOT NULL,
    reason VARCHAR(32) NOT NULL CHECK (
        reason IN (
            'user_request',
            'retention_expired',
            'authorization_revoked',
            'source_deleted'
        )
    ),
    status VARCHAR(16) NOT NULL CHECK (status IN ('pending', 'completed', 'failed')),
    requested_at TIMESTAMPTZ NOT NULL,
    cleanup_due_at TIMESTAMPTZ NOT NULL CHECK (cleanup_due_at >= requested_at),
    completed_at TIMESTAMPTZ,
    CONSTRAINT evidence_deletions_owner_operation_key UNIQUE (owner_id, operation_id),
    CONSTRAINT evidence_deletions_owner_resource_key UNIQUE (owner_id, resource_record_id),
    CONSTRAINT evidence_deletions_owner_resource_fkey
        FOREIGN KEY (owner_id, resource_record_id)
        REFERENCES evidence_resources (owner_id, id)
        ON DELETE CASCADE,
    CHECK ((status = 'completed') = (completed_at IS NOT NULL))
);

CREATE INDEX evidence_deletions_status_due_idx
    ON evidence_deletions (status, cleanup_due_at);

CREATE TABLE evidence_cleanup_targets (
    id UUID PRIMARY KEY,
    deletion_id UUID NOT NULL REFERENCES evidence_deletions (id) ON DELETE CASCADE,
    target_kind VARCHAR(32) NOT NULL CHECK (
        target_kind IN ('redis_cache', 'minio_object')
    ),
    target_reference VARCHAR(1024) NOT NULL,
    status VARCHAR(16) NOT NULL CHECK (
        status IN ('pending', 'processing', 'failed', 'succeeded')
    ),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count BETWEEN 0 AND 6),
    next_attempt_at TIMESTAMPTZ,
    lease_token UUID,
    lease_expires_at TIMESTAMPTZ,
    last_error_code VARCHAR(128),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
    CONSTRAINT evidence_cleanup_targets_deletion_kind_reference_key
        UNIQUE (deletion_id, target_kind, target_reference),
    CHECK (
        (status = 'processing') = (lease_token IS NOT NULL AND lease_expires_at IS NOT NULL)
    ),
    CHECK ((status = 'succeeded') = (completed_at IS NOT NULL))
);

CREATE INDEX evidence_cleanup_targets_claim_idx
    ON evidence_cleanup_targets (status, next_attempt_at);

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
    lease_owner VARCHAR(128),
    lease_epoch BIGINT NOT NULL DEFAULT 0 CHECK (lease_epoch >= 0),
    lease_expires_at TIMESTAMPTZ,
    checkpoint_sequence BIGINT NOT NULL DEFAULT 0 CHECK (checkpoint_sequence >= 0),
    checkpoint JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(checkpoint) = 'object'),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
    CONSTRAINT jobs_owner_kind_operation_key UNIQUE (owner_id, kind, operation_id),
    CHECK (
        (lease_owner IS NULL AND lease_expires_at IS NULL)
        OR (lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
    ),
    CHECK (status = 'running' OR (lease_owner IS NULL AND lease_expires_at IS NULL)),
    CHECK (
        completed_at IS NULL
        OR status IN ('succeeded', 'partially_succeeded', 'failed', 'cancelled')
    )
);

CREATE INDEX jobs_runnable_idx ON jobs (status, lease_expires_at);

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

CREATE INDEX outbox_messages_unpublished_idx
    ON outbox_messages (created_at)
    WHERE published_at IS NULL;

CREATE TABLE job_attempts (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    lease_epoch BIGINT NOT NULL CHECK (lease_epoch >= 1),
    worker_id VARCHAR(128) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL CHECK (lease_expires_at > started_at),
    finished_at TIMESTAMPTZ,
    outcome VARCHAR(32),
    CONSTRAINT job_attempts_job_epoch_key UNIQUE (job_id, lease_epoch),
    CHECK (
        (finished_at IS NULL AND outcome IS NULL)
        OR (finished_at IS NOT NULL AND outcome IS NOT NULL)
    ),
    CHECK (finished_at IS NULL OR finished_at >= started_at),
    CHECK (outcome IS NULL OR outcome IN ('expired', 'succeeded'))
);

CREATE TABLE processed_messages (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    topic VARCHAR(128) NOT NULL,
    partition INTEGER NOT NULL CHECK (partition >= 0),
    message_offset BIGINT NOT NULL CHECK (message_offset >= 0),
    processed_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT processed_messages_topic_partition_offset_key
        UNIQUE (topic, partition, message_offset)
);

COMMIT;
