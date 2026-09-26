from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    BigInteger,
    CheckConstraint,
    DateTime,
    ForeignKey,
    ForeignKeyConstraint,
    Index,
    Integer,
    LargeBinary,
    String,
    UniqueConstraint,
    text,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base

type JsonValue = str | int | bool | None


class ResourceBudgetPolicy(Base):
    __tablename__ = "resource_budget_policies"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "budget_key",
            name="resource_budget_policies_owner_key",
        ),
        UniqueConstraint(
            "owner_id",
            "id",
            name="resource_budget_policies_owner_id_key",
        ),
        CheckConstraint(
            "budget_key ~ '^[a-z][a-z0-9_.:-]{0,127}$'",
            name="resource_budget_policies_key_check",
        ),
        CheckConstraint(
            "metric IN ('network_request', 'collector_call', 'analysis_attempt', "
            "'concurrency_slot', 'x_api_usd_micros')",
            name="resource_budget_policies_metric_check",
        ),
        CheckConstraint(
            "scope_kind IN ('global', 'source', 'connection', 'job')",
            name="resource_budget_policies_scope_check",
        ),
        CheckConstraint(
            "(scope_kind = 'global' AND scope_reference IS NULL) OR "
            "(scope_kind <> 'global' AND "
            "scope_reference ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$')",
            name="resource_budget_policies_reference_check",
        ),
        CheckConstraint(
            "metric <> 'x_api_usd_micros' OR scope_kind <> 'source' OR scope_reference = 'x'",
            name="resource_budget_policies_x_source_check",
        ),
        CheckConstraint("limit_units > 0", name="resource_budget_policies_limit_check"),
        CheckConstraint(
            "window_seconds > 0",
            name="resource_budget_policies_window_check",
        ),
        CheckConstraint(
            "policy_version >= 1",
            name="resource_budget_policies_version_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="resource_budget_policies_updated_at_check",
        ),
        Index(
            "resource_budget_policies_source_window_key",
            "owner_id",
            "scope_reference",
            "metric",
            "window_seconds",
            "window_anchor_at",
            unique=True,
            postgresql_where=text("scope_kind = 'source'"),
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    budget_key: Mapped[str] = mapped_column(String(128))
    metric: Mapped[str] = mapped_column(String(32))
    scope_kind: Mapped[str] = mapped_column(String(32))
    scope_reference: Mapped[str | None] = mapped_column(String(128))
    limit_units: Mapped[int] = mapped_column(BigInteger)
    window_seconds: Mapped[int] = mapped_column(BigInteger)
    window_anchor_at: Mapped[datetime]
    enabled: Mapped[bool]
    policy_version: Mapped[int] = mapped_column(BigInteger, server_default=text("1"))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]


class ResourceBudgetWindow(Base):
    __tablename__ = "resource_budget_windows"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "budget_policy_id",
            "window_start",
            name="resource_budget_windows_policy_start_key",
        ),
        UniqueConstraint(
            "owner_id",
            "id",
            name="resource_budget_windows_owner_id_key",
        ),
        ForeignKeyConstraint(
            ["owner_id", "budget_policy_id"],
            ["resource_budget_policies.owner_id", "resource_budget_policies.id"],
            ondelete="CASCADE",
            name="resource_budget_windows_owner_policy_fkey",
        ),
        CheckConstraint(
            "budget_mode IN ('cumulative', 'concurrent')",
            name="resource_budget_windows_mode_check",
        ),
        CheckConstraint(
            "window_end > window_start",
            name="resource_budget_windows_bounds_check",
        ),
        CheckConstraint(
            "used_units >= 0 AND reserved_units >= 0",
            name="resource_budget_windows_units_check",
        ),
        CheckConstraint(
            "budget_mode = 'cumulative' OR used_units = 0",
            name="resource_budget_windows_concurrent_used_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="resource_budget_windows_updated_at_check",
        ),
        Index(
            "resource_budget_windows_lookup_idx",
            "owner_id",
            "budget_policy_id",
            "window_end",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    budget_policy_id: Mapped[UUID]
    budget_mode: Mapped[str] = mapped_column(String(32))
    window_start: Mapped[datetime]
    window_end: Mapped[datetime]
    used_units: Mapped[int] = mapped_column(BigInteger, server_default=text("0"))
    reserved_units: Mapped[int] = mapped_column(BigInteger, server_default=text("0"))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]


class ResourceBudgetReservation(Base):
    __tablename__ = "resource_budget_reservations"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "reservation_id",
            "budget_policy_id",
            name="resource_budget_reservations_owner_reservation_policy_key",
        ),
        ForeignKeyConstraint(
            ["owner_id", "budget_policy_id"],
            ["resource_budget_policies.owner_id", "resource_budget_policies.id"],
            ondelete="CASCADE",
            name="resource_budget_reservations_owner_policy_fkey",
        ),
        ForeignKeyConstraint(
            ["owner_id", "budget_window_id"],
            ["resource_budget_windows.owner_id", "resource_budget_windows.id"],
            ondelete="CASCADE",
            name="resource_budget_reservations_owner_window_fkey",
        ),
        CheckConstraint(
            "metric IN ('network_request', 'collector_call', 'analysis_attempt', "
            "'concurrency_slot', 'x_api_usd_micros')",
            name="resource_budget_reservations_metric_check",
        ),
        CheckConstraint(
            "budget_mode IN ('cumulative', 'concurrent')",
            name="resource_budget_reservations_mode_check",
        ),
        CheckConstraint(
            "status IN ('reserved', 'settled')",
            name="resource_budget_reservations_status_check",
        ),
        CheckConstraint(
            "requested_units > 0 AND remaining_units_after >= 0",
            name="resource_budget_reservations_units_check",
        ),
        CheckConstraint(
            "octet_length(context_fingerprint) = 32",
            name="resource_budget_reservations_fingerprint_check",
        ),
        CheckConstraint(
            "(status = 'reserved' AND actual_units IS NULL AND released_units IS NULL "
            "AND settled_at IS NULL) OR "
            "(status = 'settled' AND actual_units IS NOT NULL "
            "AND released_units IS NOT NULL AND settled_at IS NOT NULL)",
            name="resource_budget_reservations_settlement_check",
        ),
        CheckConstraint(
            "actual_units IS NULL OR (actual_units >= 0 AND actual_units <= requested_units)",
            name="resource_budget_reservations_actual_check",
        ),
        CheckConstraint(
            "released_units IS NULL OR (released_units >= 0 AND released_units <= requested_units)",
            name="resource_budget_reservations_released_check",
        ),
        CheckConstraint(
            "status = 'reserved' OR "
            "(budget_mode = 'cumulative' AND actual_units + released_units = requested_units) OR "
            "(budget_mode = 'concurrent' AND released_units = requested_units)",
            name="resource_budget_reservations_balance_check",
        ),
        Index(
            "resource_budget_reservations_lookup_idx",
            "owner_id",
            "reservation_id",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    reservation_id: Mapped[UUID]
    operation_id: Mapped[UUID]
    budget_policy_id: Mapped[UUID]
    budget_window_id: Mapped[UUID]
    policy_version: Mapped[int] = mapped_column(BigInteger)
    limit_units: Mapped[int] = mapped_column(BigInteger)
    metric: Mapped[str] = mapped_column(String(32))
    budget_mode: Mapped[str] = mapped_column(String(32))
    requested_units: Mapped[int] = mapped_column(BigInteger)
    actual_units: Mapped[int | None] = mapped_column(BigInteger)
    released_units: Mapped[int | None] = mapped_column(BigInteger)
    remaining_units_after: Mapped[int] = mapped_column(BigInteger)
    context_fingerprint: Mapped[bytes] = mapped_column(LargeBinary(32))
    status: Mapped[str] = mapped_column(String(32), server_default=text("'reserved'"))
    created_at: Mapped[datetime]
    settled_at: Mapped[datetime | None]


class ResourceComponentPolicy(Base):
    __tablename__ = "resource_component_policies"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "component_key",
            name="resource_component_policies_owner_component_key",
        ),
        UniqueConstraint(
            "owner_id",
            "id",
            name="resource_component_policies_owner_id_key",
        ),
        CheckConstraint(
            "component_key ~ '^[a-z][a-z0-9_.:-]{0,127}$'",
            name="resource_component_policies_component_key_check",
        ),
        CheckConstraint(
            "cost_class IN ('local', 'zero_price', 'free_credit', 'paid', 'unknown')",
            name="resource_component_policies_cost_class_check",
        ),
        CheckConstraint(
            "NOT enabled_for_core OR cost_class IN ('local', 'zero_price')",
            name="resource_component_policies_core_cost_check",
        ),
        CheckConstraint(
            "policy_version >= 1",
            name="resource_component_policies_version_check",
        ),
        CheckConstraint(
            "updated_at >= created_at",
            name="resource_component_policies_updated_at_check",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    component_key: Mapped[str] = mapped_column(String(128))
    component_version: Mapped[str] = mapped_column(String(128))
    cost_class: Mapped[str] = mapped_column(String(32))
    enabled_for_core: Mapped[bool]
    terms_reference: Mapped[str] = mapped_column(String(512))
    reviewed_at: Mapped[datetime]
    policy_version: Mapped[int] = mapped_column(BigInteger, server_default=text("1"))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]


class ResourceUsageAttempt(Base):
    __tablename__ = "resource_usage_attempts"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "attempt_id",
            name="resource_usage_attempts_owner_attempt_key",
        ),
        ForeignKeyConstraint(
            ["owner_id", "component_policy_id"],
            ["resource_component_policies.owner_id", "resource_component_policies.id"],
            ondelete="CASCADE",
            name="resource_usage_attempts_owner_policy_fkey",
        ),
        CheckConstraint(
            "usage_kind IN ('network_request', 'collector_call', 'analysis_attempt')",
            name="resource_usage_attempts_kind_check",
        ),
        CheckConstraint(
            "stage ~ '^[a-z][a-z0-9_.:-]{0,127}$'",
            name="resource_usage_attempts_stage_check",
        ),
        CheckConstraint(
            "outcome IN ('started', 'succeeded', 'failed', 'filtered', 'empty')",
            name="resource_usage_attempts_outcome_check",
        ),
        CheckConstraint(
            "(outcome = 'started' AND finished_at IS NULL) OR "
            "(outcome <> 'started' AND finished_at IS NOT NULL)",
            name="resource_usage_attempts_finished_pair_check",
        ),
        CheckConstraint(
            "finished_at IS NULL OR finished_at >= started_at",
            name="resource_usage_attempts_finished_at_check",
        ),
        Index(
            "resource_usage_attempts_operation_idx",
            "owner_id",
            "operation_id",
            "started_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    attempt_id: Mapped[UUID]
    operation_id: Mapped[UUID]
    component_policy_id: Mapped[UUID]
    component_version: Mapped[str] = mapped_column(String(128))
    usage_kind: Mapped[str] = mapped_column(String(32))
    stage: Mapped[str] = mapped_column(String(128))
    outcome: Mapped[str] = mapped_column(String(32), server_default=text("'started'"))
    started_at: Mapped[datetime]
    finished_at: Mapped[datetime | None]


class Job(Base):
    __tablename__ = "jobs"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "kind",
            "operation_id",
            name="jobs_owner_kind_operation_key",
        ),
        UniqueConstraint("owner_id", "id", name="jobs_owner_id_key"),
        CheckConstraint(
            "kind ~ '^[a-z][a-z0-9_.-]{0,63}$'",
            name="jobs_kind_check",
        ),
        CheckConstraint(
            "status IN ('queued', 'running', 'succeeded', "
            "'partially_succeeded', 'failed', 'cancelled')",
            name="jobs_status_check",
        ),
        CheckConstraint(
            "configuration_ref ~ '^[a-z0-9][a-z0-9_.:-]{0,127}$'",
            name="jobs_configuration_ref_check",
        ),
        CheckConstraint(
            "configuration_version >= 1",
            name="jobs_configuration_version_check",
        ),
        CheckConstraint(
            "(source_key IS NULL AND source_capability IS NULL) OR "
            "(source_key IS NOT NULL AND source_capability IS NOT NULL AND "
            "source_key ~ '^[a-z][a-z0-9_-]{0,63}$' AND "
            "source_capability IN "
            "('search', 'author_posts', 'comments', 'replies', 'page_content', 'hotlist'))",
            name="jobs_source_context_check",
        ),
        CheckConstraint(
            "octet_length(request_fingerprint) = 32",
            name="jobs_request_fingerprint_length_check",
        ),
        CheckConstraint("jsonb_typeof(scope) = 'object'", name="jobs_scope_object_check"),
        CheckConstraint(
            "jsonb_typeof(checkpoint) = 'object'",
            name="jobs_checkpoint_object_check",
        ),
        CheckConstraint("lease_epoch >= 0", name="jobs_lease_epoch_check"),
        CheckConstraint(
            "checkpoint_sequence >= 0",
            name="jobs_checkpoint_sequence_check",
        ),
        CheckConstraint(
            "progress_stage IS NULL OR progress_stage IN ('request', 'parse', 'save', 'analysis')",
            name="jobs_progress_stage_check",
        ),
        CheckConstraint("requests_sent >= 0", name="jobs_requests_sent_check"),
        CheckConstraint("items_saved >= 0", name="jobs_items_saved_check"),
        CheckConstraint(
            "(progress_stage IS NULL AND progress_updated_at IS NULL AND "
            "requests_sent = 0 AND items_saved = 0) OR "
            "(progress_stage IS NOT NULL AND progress_updated_at IS NOT NULL)",
            name="jobs_progress_pair_check",
        ),
        CheckConstraint(
            "progress_updated_at IS NULL OR progress_updated_at >= created_at",
            name="jobs_progress_updated_at_check",
        ),
        CheckConstraint(
            "(cancel_requested_at IS NULL AND cancel_deadline_at IS NULL) OR "
            "(cancel_requested_at IS NOT NULL AND status IN ('running', 'cancelled') AND "
            "(cancel_deadline_at IS NULL OR cancel_deadline_at >= cancel_requested_at))",
            name="jobs_cancel_request_check",
        ),
        CheckConstraint(
            "(lease_owner IS NULL AND lease_expires_at IS NULL) OR "
            "(lease_owner IS NOT NULL AND lease_expires_at IS NOT NULL)",
            name="jobs_lease_pair_check",
        ),
        CheckConstraint(
            "status = 'running' OR (lease_owner IS NULL AND lease_expires_at IS NULL)",
            name="jobs_non_running_lease_check",
        ),
        CheckConstraint(
            "completed_at IS NULL OR status IN "
            "('succeeded', 'partially_succeeded', 'failed', 'cancelled')",
            name="jobs_completed_status_check",
        ),
        CheckConstraint(
            "(defer_reason IS NULL AND next_run_at IS NULL) OR "
            "(status = 'queued' AND defer_reason IS NOT NULL AND next_run_at IS NOT NULL AND "
            "defer_reason ~ '^[a-z][a-z0-9_.:-]{0,127}$')",
            name="jobs_defer_pair_check",
        ),
        CheckConstraint(
            "next_run_at IS NULL OR next_run_at >= created_at",
            name="jobs_next_run_at_check",
        ),
        CheckConstraint("retry_count >= 0", name="jobs_retry_count_check"),
        CheckConstraint(
            "(last_error_code IS NULL AND last_error_category IS NULL AND "
            "last_error_at IS NULL AND next_action IS NULL) OR "
            "(last_error_code ~ '^[a-z][a-z0-9_.:-]{0,127}$' AND "
            "last_error_category IN ('transient', 'rate_limited', "
            "'authentication_required', 'permission_denied', 'invalid_response', "
            "'parse_error', 'invalid_input', 'configuration_unavailable') AND "
            "last_error_at IS NOT NULL AND next_action IS NOT NULL)",
            name="jobs_failure_context_check",
        ),
        CheckConstraint("updated_at >= created_at", name="jobs_updated_at_check"),
        Index("jobs_runnable_idx", "status", "lease_expires_at"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    operation_id: Mapped[UUID]
    kind: Mapped[str] = mapped_column(String(64))
    configuration_ref: Mapped[str] = mapped_column(String(128))
    configuration_version: Mapped[int] = mapped_column(BigInteger)
    source_key: Mapped[str | None] = mapped_column(String(64))
    source_capability: Mapped[str | None] = mapped_column(String(32))
    scope: Mapped[dict[str, JsonValue]] = mapped_column(JSONB)
    request_fingerprint: Mapped[bytes] = mapped_column(LargeBinary(32))
    status: Mapped[str] = mapped_column(String(32), server_default=text("'queued'"))
    lease_owner: Mapped[str | None] = mapped_column(String(128))
    lease_epoch: Mapped[int] = mapped_column(BigInteger, server_default=text("0"))
    lease_expires_at: Mapped[datetime | None]
    checkpoint_sequence: Mapped[int] = mapped_column(BigInteger, server_default=text("0"))
    checkpoint: Mapped[dict[str, JsonValue]] = mapped_column(
        JSONB,
        server_default=text("'{}'::jsonb"),
    )
    progress_stage: Mapped[str | None] = mapped_column(String(32))
    requests_sent: Mapped[int] = mapped_column(BigInteger, server_default=text("0"))
    items_saved: Mapped[int] = mapped_column(BigInteger, server_default=text("0"))
    progress_updated_at: Mapped[datetime | None]
    cancel_requested_at: Mapped[datetime | None]
    cancel_deadline_at: Mapped[datetime | None]
    scheduled_for_at: Mapped[datetime | None]
    started_at: Mapped[datetime | None]
    completed_at: Mapped[datetime | None]
    defer_reason: Mapped[str | None] = mapped_column(String(128))
    next_run_at: Mapped[datetime | None]
    retry_count: Mapped[int] = mapped_column(BigInteger, server_default=text("0"))
    last_error_code: Mapped[str | None] = mapped_column(String(128))
    last_error_category: Mapped[str | None] = mapped_column(String(32))
    last_error_at: Mapped[datetime | None]
    next_action: Mapped[str | None] = mapped_column(String(512))
    manual_retry_allowed: Mapped[bool] = mapped_column(server_default=text("false"))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]


class CoverageWindow(Base):
    __tablename__ = "coverage_windows"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "source_key",
            "capability",
            "target_hash",
            "sort_key",
            "rule_version",
            "starts_at",
            "ends_at",
            name="coverage_windows_scope_range_key",
        ),
        ForeignKeyConstraint(
            ["owner_id", "last_job_id"],
            ["jobs.owner_id", "jobs.id"],
            ondelete="SET NULL (last_job_id)",
            name="coverage_windows_owner_job_fkey",
        ),
        CheckConstraint(
            "source_key ~ '^[a-z][a-z0-9_-]{0,63}$'", name="coverage_windows_source_check"
        ),
        CheckConstraint(
            "capability IN ('search', 'author_posts', 'comments', 'replies')",
            name="coverage_windows_capability_check",
        ),
        CheckConstraint(
            "octet_length(target_hash) = 32", name="coverage_windows_target_hash_check"
        ),
        CheckConstraint("sort_key IN ('latest', 'top')", name="coverage_windows_sort_check"),
        CheckConstraint("rule_version >= 1", name="coverage_windows_rule_version_check"),
        CheckConstraint("starts_at < ends_at", name="coverage_windows_range_check"),
        CheckConstraint(
            "status IN ('pending', 'running', 'confirmed', 'partial')",
            name="coverage_windows_status_check",
        ),
        CheckConstraint(
            "(status = 'partial' AND stop_reason ~ '^[a-z][a-z0-9_]{0,63}$') OR "
            "(status <> 'partial' AND stop_reason IS NULL)",
            name="coverage_windows_reason_check",
        ),
        CheckConstraint("checkpoint_sequence >= 0", name="coverage_windows_checkpoint_check"),
        CheckConstraint("page_count >= 0", name="coverage_windows_page_count_check"),
        CheckConstraint("updated_at >= created_at", name="coverage_windows_updated_at_check"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    source_key: Mapped[str] = mapped_column(String(64))
    capability: Mapped[str] = mapped_column(String(32))
    target_hash: Mapped[bytes] = mapped_column(LargeBinary(32))
    sort_key: Mapped[str] = mapped_column(String(16))
    rule_version: Mapped[int] = mapped_column(BigInteger)
    starts_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    ends_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    status: Mapped[str] = mapped_column(String(16), server_default=text("'pending'"))
    stop_reason: Mapped[str | None] = mapped_column(String(64))
    last_job_id: Mapped[UUID | None]
    checkpoint_sequence: Mapped[int] = mapped_column(BigInteger, server_default=text("0"))
    page_count: Mapped[int] = mapped_column(BigInteger, server_default=text("0"))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


class CollectionDueWindow(Base):
    __tablename__ = "collection_due_windows"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "schedule_key",
            "due_at",
            name="collection_due_windows_owner_schedule_due_key",
        ),
        ForeignKeyConstraint(
            ["owner_id", "topic_id"],
            ["monitor_topics.owner_id", "monitor_topics.id"],
            name="collection_due_windows_owner_topic_fkey",
            deferrable=True,
            initially="DEFERRED",
        ),
        ForeignKeyConstraint(
            ["owner_id", "job_id"],
            ["jobs.owner_id", "jobs.id"],
            name="collection_due_windows_owner_job_fkey",
            deferrable=True,
            initially="DEFERRED",
        ),
        CheckConstraint(
            "source_key ~ '^[a-z][a-z0-9_-]{0,63}$'",
            name="collection_due_windows_source_check",
        ),
        CheckConstraint(
            "capability IN ('search', 'author_posts', 'comments', 'replies', "
            "'page_content', 'hotlist')",
            name="collection_due_windows_capability_check",
        ),
        CheckConstraint(
            "window_start < window_end AND window_end <= due_at",
            name="collection_due_windows_range_check",
        ),
        CheckConstraint(
            "connection_version IS NULL OR connection_version >= 1",
            name="collection_due_windows_version_check",
        ),
        CheckConstraint(
            "policy_snapshot IS NULL OR jsonb_typeof(policy_snapshot) = 'object'",
            name="collection_due_windows_policy_object_check",
        ),
        CheckConstraint(
            "admission_state IN ('pending', 'accepted', 'skipped', 'missed')",
            name="collection_due_windows_state_check",
        ),
        CheckConstraint(
            "(admission_state = 'pending' AND reason IS NULL AND operation_id IS NULL "
            "AND job_id IS NULL) OR "
            "(admission_state = 'accepted' AND reason IS NULL AND operation_id IS NOT NULL "
            "AND job_id IS NOT NULL) OR "
            "(admission_state = 'skipped' AND reason IS NOT NULL AND reason IN "
            "('quiet', 'disabled', 'rate_limited', 'budget') "
            "AND operation_id IS NULL AND job_id IS NULL) OR "
            "(admission_state = 'missed' AND reason IS NOT NULL "
            "AND reason = 'scheduler_interrupted' "
            "AND operation_id IS NULL AND job_id IS NULL)",
            name="collection_due_windows_admission_pair_check",
        ),
        Index("collection_due_windows_owner_due_idx", "owner_id", "due_at"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    schedule_key: Mapped[UUID]
    topic_id: Mapped[UUID | None]
    source_key: Mapped[str] = mapped_column(String(64))
    capability: Mapped[str] = mapped_column(String(32))
    due_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    window_start: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    window_end: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    connection_version: Mapped[int | None] = mapped_column(BigInteger)
    policy_snapshot: Mapped[dict[str, object] | None] = mapped_column(JSONB)
    admission_state: Mapped[str] = mapped_column(String(16), server_default=text("'pending'"))
    reason: Mapped[str | None] = mapped_column(String(64))
    operation_id: Mapped[UUID | None]
    job_id: Mapped[UUID | None]
    recorded_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


class JobStageAttempt(Base):
    __tablename__ = "job_stage_attempts"
    __table_args__ = (
        UniqueConstraint(
            "job_id",
            "stage",
            "attempt_sequence",
            name="job_stage_attempts_job_stage_sequence_key",
        ),
        ForeignKeyConstraint(
            ["owner_id", "job_id"],
            ["jobs.owner_id", "jobs.id"],
            ondelete="CASCADE",
            name="job_stage_attempts_owner_job_fkey",
        ),
        CheckConstraint(
            "stage IN ('request', 'parse', 'save', 'analysis')",
            name="job_stage_attempts_stage_check",
        ),
        CheckConstraint(
            "attempt_sequence >= 1",
            name="job_stage_attempts_sequence_check",
        ),
        CheckConstraint(
            "outcome IN ('started', 'succeeded', 'partially_succeeded', "
            "'failed', 'delayed', 'cancelled')",
            name="job_stage_attempts_outcome_check",
        ),
        CheckConstraint(
            "(outcome = 'started' AND finished_at IS NULL) OR "
            "(outcome <> 'started' AND finished_at IS NOT NULL)",
            name="job_stage_attempts_finished_pair_check",
        ),
        CheckConstraint(
            "finished_at IS NULL OR finished_at >= started_at",
            name="job_stage_attempts_finished_at_check",
        ),
        Index("job_stage_attempts_owner_started_idx", "owner_id", "started_at"),
        Index("job_stage_attempts_job_stage_idx", "job_id", "stage", "attempt_sequence"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID]
    job_id: Mapped[UUID]
    stage: Mapped[str] = mapped_column(String(32))
    attempt_sequence: Mapped[int] = mapped_column(BigInteger)
    outcome: Mapped[str] = mapped_column(String(32), server_default=text("'started'"))
    started_at: Mapped[datetime]
    finished_at: Mapped[datetime | None]


class OutboxMessage(Base):
    __tablename__ = "outbox_messages"
    __table_args__ = (
        UniqueConstraint(
            "aggregate_id",
            "dispatch_sequence",
            name="outbox_messages_aggregate_dispatch_key",
        ),
        CheckConstraint("dispatch_sequence >= 1", name="outbox_messages_dispatch_check"),
        CheckConstraint(
            "jsonb_typeof(payload) = 'object'",
            name="outbox_messages_payload_object_check",
        ),
        CheckConstraint(
            "published_at IS NULL OR published_at >= created_at",
            name="outbox_messages_published_at_check",
        ),
        Index(
            "outbox_messages_unpublished_idx",
            "available_at",
            postgresql_where=text("published_at IS NULL"),
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    aggregate_id: Mapped[UUID] = mapped_column(ForeignKey("jobs.id", ondelete="CASCADE"))
    topic: Mapped[str] = mapped_column(String(128))
    message_key: Mapped[UUID]
    event_type: Mapped[str] = mapped_column(String(64))
    dispatch_sequence: Mapped[int] = mapped_column(BigInteger)
    payload: Mapped[dict[str, JsonValue]] = mapped_column(JSONB)
    available_at: Mapped[datetime]
    created_at: Mapped[datetime]
    published_at: Mapped[datetime | None]


class JobAttempt(Base):
    __tablename__ = "job_attempts"
    __table_args__ = (
        UniqueConstraint("job_id", "lease_epoch", name="job_attempts_job_epoch_key"),
        CheckConstraint("lease_epoch >= 1", name="job_attempts_lease_epoch_check"),
        CheckConstraint(
            "lease_expires_at > started_at",
            name="job_attempts_lease_expiry_check",
        ),
        CheckConstraint(
            "(finished_at IS NULL AND outcome IS NULL) OR "
            "(finished_at IS NOT NULL AND outcome IS NOT NULL)",
            name="job_attempts_outcome_pair_check",
        ),
        CheckConstraint(
            "finished_at IS NULL OR finished_at >= started_at",
            name="job_attempts_finished_at_check",
        ),
        CheckConstraint(
            "outcome IS NULL OR outcome IN "
            "('expired', 'succeeded', 'cancelled', 'delayed', 'failed')",
            name="job_attempts_outcome_check",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    job_id: Mapped[UUID] = mapped_column(ForeignKey("jobs.id", ondelete="CASCADE"))
    lease_epoch: Mapped[int] = mapped_column(BigInteger)
    worker_id: Mapped[str] = mapped_column(String(128))
    started_at: Mapped[datetime]
    lease_expires_at: Mapped[datetime]
    finished_at: Mapped[datetime | None]
    outcome: Mapped[str | None] = mapped_column(String(32))


class ProcessedMessage(Base):
    __tablename__ = "processed_messages"
    __table_args__ = (
        UniqueConstraint(
            "topic",
            "partition",
            "message_offset",
            name="processed_messages_topic_partition_offset_key",
        ),
        CheckConstraint("partition >= 0", name="processed_messages_partition_check"),
        CheckConstraint(
            "message_offset >= 0",
            name="processed_messages_offset_check",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    job_id: Mapped[UUID] = mapped_column(ForeignKey("jobs.id", ondelete="CASCADE"))
    topic: Mapped[str] = mapped_column(String(128))
    partition: Mapped[int] = mapped_column(Integer)
    message_offset: Mapped[int] = mapped_column(BigInteger)
    processed_at: Mapped[datetime]
