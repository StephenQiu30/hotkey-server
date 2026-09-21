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

CREATE TABLE monitor_topics (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    name VARCHAR(80) NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    status VARCHAR(16) NOT NULL DEFAULT 'paused'
        CHECK (status IN ('paused', 'active', 'archived')),
    readiness_status VARCHAR(32) NOT NULL DEFAULT 'pending_source_selection'
        CHECK (
            readiness_status IN (
                'pending_source_selection',
                'pending_source_readiness',
                'ready'
            )
        ),
    current_version INTEGER NOT NULL DEFAULT 1 CHECK (current_version >= 1),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at)
);

CREATE INDEX monitor_topics_owner_updated_idx ON monitor_topics (owner_id, updated_at);

CREATE TABLE monitor_topic_versions (
    topic_id UUID NOT NULL REFERENCES monitor_topics (id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version >= 1),
    created_by UUID NOT NULL REFERENCES identity_users (id) ON DELETE RESTRICT,
    match_any JSONB NOT NULL CHECK (jsonb_typeof(match_any) = 'array'),
    match_all JSONB NOT NULL CHECK (jsonb_typeof(match_all) = 'array'),
    exclude JSONB NOT NULL CHECK (jsonb_typeof(exclude) = 'array'),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (topic_id, version)
);

CREATE INDEX monitor_topic_versions_created_by_idx
    ON monitor_topic_versions (created_by);

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

CREATE TABLE resource_budget_policies (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    budget_key VARCHAR(128) NOT NULL CHECK (
        budget_key ~ '^[a-z][a-z0-9_.:-]{0,127}$'
    ),
    metric VARCHAR(32) NOT NULL CHECK (
        metric IN ('network_request', 'analysis_attempt', 'concurrency_slot')
    ),
    scope_kind VARCHAR(32) NOT NULL CHECK (
        scope_kind IN ('global', 'source', 'connection', 'job')
    ),
    scope_reference VARCHAR(128),
    limit_units BIGINT NOT NULL CHECK (limit_units > 0),
    window_seconds BIGINT NOT NULL CHECK (window_seconds > 0),
    window_anchor_at TIMESTAMPTZ NOT NULL,
    enabled BOOLEAN NOT NULL,
    policy_version BIGINT NOT NULL DEFAULT 1 CHECK (policy_version >= 1),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
    CONSTRAINT resource_budget_policies_owner_key UNIQUE (owner_id, budget_key),
    CONSTRAINT resource_budget_policies_owner_id_key UNIQUE (owner_id, id),
    CHECK (
        (scope_kind = 'global' AND scope_reference IS NULL)
        OR (
            scope_kind <> 'global'
            AND scope_reference ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$'
        )
    )
);

CREATE TABLE resource_budget_windows (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL,
    budget_policy_id UUID NOT NULL,
    budget_mode VARCHAR(32) NOT NULL CHECK (
        budget_mode IN ('cumulative', 'concurrent')
    ),
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL CHECK (window_end > window_start),
    used_units BIGINT NOT NULL DEFAULT 0 CHECK (used_units >= 0),
    reserved_units BIGINT NOT NULL DEFAULT 0 CHECK (reserved_units >= 0),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
    CONSTRAINT resource_budget_windows_policy_start_key
        UNIQUE (owner_id, budget_policy_id, window_start),
    CONSTRAINT resource_budget_windows_owner_id_key UNIQUE (owner_id, id),
    CONSTRAINT resource_budget_windows_owner_policy_fkey
        FOREIGN KEY (owner_id, budget_policy_id)
        REFERENCES resource_budget_policies (owner_id, id) ON DELETE CASCADE,
    CHECK (budget_mode = 'cumulative' OR used_units = 0)
);

CREATE INDEX resource_budget_windows_lookup_idx
    ON resource_budget_windows (owner_id, budget_policy_id, window_end);

CREATE TABLE resource_budget_reservations (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL,
    reservation_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    budget_policy_id UUID NOT NULL,
    budget_window_id UUID NOT NULL,
    policy_version BIGINT NOT NULL CHECK (policy_version >= 1),
    limit_units BIGINT NOT NULL CHECK (limit_units > 0),
    metric VARCHAR(32) NOT NULL CHECK (
        metric IN ('network_request', 'analysis_attempt', 'concurrency_slot')
    ),
    budget_mode VARCHAR(32) NOT NULL CHECK (
        budget_mode IN ('cumulative', 'concurrent')
    ),
    requested_units BIGINT NOT NULL CHECK (requested_units > 0),
    actual_units BIGINT,
    released_units BIGINT,
    remaining_units_after BIGINT NOT NULL CHECK (remaining_units_after >= 0),
    context_fingerprint BYTEA NOT NULL CHECK (octet_length(context_fingerprint) = 32),
    status VARCHAR(32) NOT NULL DEFAULT 'reserved' CHECK (
        status IN ('reserved', 'settled')
    ),
    created_at TIMESTAMPTZ NOT NULL,
    settled_at TIMESTAMPTZ,
    CONSTRAINT resource_budget_reservations_owner_reservation_policy_key
        UNIQUE (owner_id, reservation_id, budget_policy_id),
    CONSTRAINT resource_budget_reservations_owner_policy_fkey
        FOREIGN KEY (owner_id, budget_policy_id)
        REFERENCES resource_budget_policies (owner_id, id) ON DELETE CASCADE,
    CONSTRAINT resource_budget_reservations_owner_window_fkey
        FOREIGN KEY (owner_id, budget_window_id)
        REFERENCES resource_budget_windows (owner_id, id) ON DELETE CASCADE,
    CHECK (
        (status = 'reserved' AND actual_units IS NULL AND released_units IS NULL
            AND settled_at IS NULL)
        OR (status = 'settled' AND actual_units IS NOT NULL
            AND released_units IS NOT NULL AND settled_at IS NOT NULL)
    ),
    CHECK (
        actual_units IS NULL
        OR (actual_units >= 0 AND actual_units <= requested_units)
    ),
    CHECK (
        released_units IS NULL
        OR (released_units >= 0 AND released_units <= requested_units)
    ),
    CHECK (
        status = 'reserved'
        OR (budget_mode = 'cumulative' AND actual_units + released_units = requested_units)
        OR (budget_mode = 'concurrent' AND released_units = requested_units)
    )
);

CREATE INDEX resource_budget_reservations_lookup_idx
    ON resource_budget_reservations (owner_id, reservation_id);

CREATE TABLE resource_component_policies (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    component_key VARCHAR(128) NOT NULL CHECK (
        component_key ~ '^[a-z][a-z0-9_.:-]{0,127}$'
    ),
    component_version VARCHAR(128) NOT NULL CHECK (component_version <> ''),
    cost_class VARCHAR(32) NOT NULL CHECK (
        cost_class IN ('local', 'zero_price', 'free_credit', 'paid', 'unknown')
    ),
    enabled_for_core BOOLEAN NOT NULL,
    terms_reference VARCHAR(512) NOT NULL CHECK (terms_reference <> ''),
    reviewed_at TIMESTAMPTZ NOT NULL,
    policy_version BIGINT NOT NULL DEFAULT 1 CHECK (policy_version >= 1),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
    CONSTRAINT resource_component_policies_owner_component_key
        UNIQUE (owner_id, component_key),
    CONSTRAINT resource_component_policies_owner_id_key UNIQUE (owner_id, id),
    CHECK (NOT enabled_for_core OR cost_class IN ('local', 'zero_price'))
);

CREATE TABLE resource_usage_attempts (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL,
    attempt_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    component_policy_id UUID NOT NULL,
    component_version VARCHAR(128) NOT NULL CHECK (component_version <> ''),
    usage_kind VARCHAR(32) NOT NULL CHECK (
        usage_kind IN ('network_request', 'analysis_attempt')
    ),
    stage VARCHAR(128) NOT NULL CHECK (stage ~ '^[a-z][a-z0-9_.:-]{0,127}$'),
    outcome VARCHAR(32) NOT NULL DEFAULT 'started' CHECK (
        outcome IN ('started', 'succeeded', 'failed', 'filtered', 'empty')
    ),
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    CONSTRAINT resource_usage_attempts_owner_attempt_key UNIQUE (owner_id, attempt_id),
    CONSTRAINT resource_usage_attempts_owner_policy_fkey
        FOREIGN KEY (owner_id, component_policy_id)
        REFERENCES resource_component_policies (owner_id, id) ON DELETE CASCADE,
    CHECK (
        (outcome = 'started' AND finished_at IS NULL)
        OR (outcome <> 'started' AND finished_at IS NOT NULL)
    ),
    CHECK (finished_at IS NULL OR finished_at >= started_at)
);

CREATE INDEX resource_usage_attempts_operation_idx
    ON resource_usage_attempts (owner_id, operation_id, started_at);

CREATE TABLE jobs (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    operation_id UUID NOT NULL,
    kind VARCHAR(64) NOT NULL CHECK (kind ~ '^[a-z][a-z0-9_.-]{0,63}$'),
    configuration_ref VARCHAR(128) NOT NULL
        CHECK (configuration_ref ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$'),
    configuration_version BIGINT NOT NULL CHECK (configuration_version >= 1),
    source_key VARCHAR(64),
    source_capability VARCHAR(32),
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
    progress_stage VARCHAR(32),
    requests_sent BIGINT NOT NULL DEFAULT 0 CHECK (requests_sent >= 0),
    items_saved BIGINT NOT NULL DEFAULT 0 CHECK (items_saved >= 0),
    progress_updated_at TIMESTAMPTZ,
    cancel_requested_at TIMESTAMPTZ,
    cancel_deadline_at TIMESTAMPTZ,
    scheduled_for_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    defer_reason VARCHAR(128),
    next_run_at TIMESTAMPTZ,
    retry_count BIGINT NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    last_error_code VARCHAR(128),
    last_error_category VARCHAR(32),
    last_error_at TIMESTAMPTZ,
    next_action VARCHAR(512),
    manual_retry_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL CHECK (updated_at >= created_at),
    CONSTRAINT jobs_owner_kind_operation_key UNIQUE (owner_id, kind, operation_id),
    CONSTRAINT jobs_owner_id_key UNIQUE (owner_id, id),
    CHECK (
        (source_key IS NULL AND source_capability IS NULL)
        OR (
            source_key IS NOT NULL
            AND source_capability IS NOT NULL
            AND source_key ~ '^[a-z][a-z0-9_-]{0,63}$'
            AND source_capability IN ('search', 'author_posts', 'comments', 'replies')
        )
    ),
    CHECK (
        (lease_owner IS NULL AND lease_expires_at IS NULL)
        OR (lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)
    ),
    CHECK (status = 'running' OR (lease_owner IS NULL AND lease_expires_at IS NULL)),
    CHECK (progress_stage IS NULL OR progress_stage IN ('request', 'parse', 'save', 'analysis')),
    CHECK (
        (
            progress_stage IS NULL
            AND progress_updated_at IS NULL
            AND requests_sent = 0
            AND items_saved = 0
        )
        OR (progress_stage IS NOT NULL AND progress_updated_at IS NOT NULL)
    ),
    CHECK (progress_updated_at IS NULL OR progress_updated_at >= created_at),
    CHECK (
        (cancel_requested_at IS NULL AND cancel_deadline_at IS NULL)
        OR (
            cancel_requested_at IS NOT NULL
            AND status IN ('running', 'cancelled')
            AND (cancel_deadline_at IS NULL OR cancel_deadline_at >= cancel_requested_at)
        )
    ),
    CHECK (
        completed_at IS NULL
        OR status IN ('succeeded', 'partially_succeeded', 'failed', 'cancelled')
    ),
    CHECK (
        (defer_reason IS NULL AND next_run_at IS NULL)
        OR (
            status = 'queued'
            AND defer_reason IS NOT NULL
            AND next_run_at IS NOT NULL
            AND defer_reason ~ '^[a-z][a-z0-9_.:-]{0,127}$'
        )
    ),
    CHECK (next_run_at IS NULL OR next_run_at >= created_at)
    ,CHECK (
        (
            last_error_code IS NULL
            AND last_error_category IS NULL
            AND last_error_at IS NULL
            AND next_action IS NULL
        )
        OR (
            last_error_code ~ '^[a-z][a-z0-9_.:-]{0,127}$'
            AND last_error_category IN (
                'transient',
                'rate_limited',
                'authentication_required',
                'permission_denied',
                'invalid_response',
                'parse_error',
                'invalid_input',
                'configuration_unavailable'
            )
            AND last_error_at IS NOT NULL
            AND next_action IS NOT NULL
        )
    )
);

CREATE INDEX jobs_runnable_idx ON jobs (status, lease_expires_at);

CREATE TABLE job_stage_attempts (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL,
    job_id UUID NOT NULL,
    stage VARCHAR(32) NOT NULL CHECK (stage IN ('request', 'parse', 'save', 'analysis')),
    attempt_sequence BIGINT NOT NULL CHECK (attempt_sequence >= 1),
    outcome VARCHAR(32) NOT NULL DEFAULT 'started' CHECK (
        outcome IN (
            'started',
            'succeeded',
            'partially_succeeded',
            'failed',
            'delayed',
            'cancelled'
        )
    ),
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    CONSTRAINT job_stage_attempts_job_stage_sequence_key
        UNIQUE (job_id, stage, attempt_sequence),
    CONSTRAINT job_stage_attempts_owner_job_fkey
        FOREIGN KEY (owner_id, job_id)
        REFERENCES jobs (owner_id, id)
        ON DELETE CASCADE,
    CHECK (
        (outcome = 'started' AND finished_at IS NULL)
        OR (outcome <> 'started' AND finished_at IS NOT NULL)
    ),
    CHECK (finished_at IS NULL OR finished_at >= started_at)
);

CREATE INDEX job_stage_attempts_owner_started_idx
    ON job_stage_attempts (owner_id, started_at);

CREATE INDEX job_stage_attempts_job_stage_idx
    ON job_stage_attempts (job_id, stage, attempt_sequence);

CREATE TABLE provenance_manifests (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL REFERENCES identity_users (id) ON DELETE CASCADE,
    job_id UUID NOT NULL,
    operation_id UUID NOT NULL,
    result_kind VARCHAR(64) NOT NULL CHECK (
        result_kind ~ '^[a-z][a-z0-9_.:-]{0,63}$'
    ),
    method_key VARCHAR(128) NOT NULL CHECK (
        method_key ~ '^[a-z][a-z0-9_.:-]{0,127}$'
    ),
    method_version VARCHAR(128) NOT NULL CHECK (
        method_version ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$'
    ),
    method_parameters JSONB NOT NULL CHECK (jsonb_typeof(method_parameters) = 'object'),
    manifest_fingerprint BYTEA NOT NULL CHECK (octet_length(manifest_fingerprint) = 32),
    created_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT provenance_manifests_owner_id_id_key UNIQUE (owner_id, id),
    CONSTRAINT provenance_manifests_owner_job_result_key
        UNIQUE (owner_id, job_id, result_kind),
    CONSTRAINT provenance_manifests_owner_job_fkey
        FOREIGN KEY (owner_id, job_id)
        REFERENCES jobs (owner_id, id) ON DELETE CASCADE
);

CREATE TABLE provenance_manifest_inputs (
    id UUID PRIMARY KEY,
    owner_id UUID NOT NULL,
    manifest_id UUID NOT NULL,
    role VARCHAR(16) NOT NULL CHECK (role IN ('subject', 'reference')),
    resource_record_id UUID NOT NULL,
    snapshot_ref VARCHAR(128) NOT NULL CHECK (
        snapshot_ref ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$'
    ),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    CONSTRAINT provenance_manifest_inputs_role_ordinal_key
        UNIQUE (manifest_id, role, ordinal),
    CONSTRAINT provenance_manifest_inputs_resource_snapshot_key
        UNIQUE (manifest_id, role, resource_record_id, snapshot_ref),
    CONSTRAINT provenance_manifest_inputs_owner_manifest_fkey
        FOREIGN KEY (owner_id, manifest_id)
        REFERENCES provenance_manifests (owner_id, id) ON DELETE CASCADE,
    CONSTRAINT provenance_manifest_inputs_owner_resource_fkey
        FOREIGN KEY (owner_id, resource_record_id)
        REFERENCES evidence_resources (owner_id, id)
);

CREATE INDEX provenance_manifest_inputs_manifest_idx
    ON provenance_manifest_inputs (manifest_id, role, ordinal);

CREATE TABLE outbox_messages (
    id UUID PRIMARY KEY,
    aggregate_id UUID NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    topic VARCHAR(128) NOT NULL,
    message_key UUID NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    dispatch_sequence BIGINT NOT NULL CHECK (dispatch_sequence >= 1),
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    available_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,
    CONSTRAINT outbox_messages_aggregate_dispatch_key UNIQUE (aggregate_id, dispatch_sequence),
    CHECK (published_at IS NULL OR published_at >= created_at)
);

CREATE INDEX outbox_messages_unpublished_idx
    ON outbox_messages (available_at)
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
    CHECK (
        outcome IS NULL
        OR outcome IN ('expired', 'succeeded', 'cancelled', 'delayed', 'failed')
    )
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
