from __future__ import annotations

import re
from datetime import datetime, timedelta
from enum import StrEnum
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from connections.schemas import SourceExecutionPolicy
from sources.contracts import SocialSourceCapability, SourceCapability, SourceSort, WebPageRequest

type JobScopeValue = str | int | bool | None

_SCOPE_KEY_PATTERN = re.compile(r"^[a-z][a-z0-9_.:-]{0,63}$")
_STABLE_REFERENCE_PATTERN = re.compile(r"^[a-z0-9][a-z0-9_.:-]{0,127}$")
_MAX_SCOPE_ITEMS = 32
_MAX_BUDGET_UNITS = 2**63 - 1


class JobStatus(StrEnum):
    QUEUED = "queued"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    PARTIALLY_SUCCEEDED = "partially_succeeded"
    FAILED = "failed"
    CANCELLED = "cancelled"


class JobReliabilityOutcome(StrEnum):
    SUCCEEDED = "succeeded"
    PARTIALLY_SUCCEEDED = "partially_succeeded"
    SOURCE_FAILURE = "source_failure"
    UNATTRIBUTED_FAILURE = "unattributed_failure"
    CANCELLED = "cancelled"
    IN_PROGRESS = "in_progress"


class CoverageWindowInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    owner_id: UUID
    source_key: str = Field(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_-]{0,63}$")
    capability: SocialSourceCapability
    target_hash: bytes = Field(min_length=32, max_length=32)
    sort_key: SourceSort
    rule_version: int = Field(ge=1)
    starts_at: datetime
    ends_at: datetime

    @field_validator("starts_at", "ends_at")
    @classmethod
    def require_utc(cls, value: datetime) -> datetime:
        if value.utcoffset() != timedelta(0):
            raise ValueError("coverage window bounds must be UTC")
        return value

    @model_validator(mode="after")
    def require_forward_range(self) -> CoverageWindowInput:
        if self.starts_at >= self.ends_at:
            raise ValueError("coverage window end must follow start")
        return self


class CoverageTerminalEvidence(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    starts_at: datetime
    ends_at: datetime
    sort_key: SourceSort
    query_bounded: bool
    sort_applied: bool
    terminal_verified: bool

    @field_validator("starts_at", "ends_at")
    @classmethod
    def require_utc(cls, value: datetime) -> datetime:
        if value.utcoffset() != timedelta(0):
            raise ValueError("coverage evidence bounds must be UTC")
        return value


class CoverageWindowStatus(StrEnum):
    PENDING = "pending"
    RUNNING = "running"
    CONFIRMED = "confirmed"
    PARTIAL = "partial"


class CollectionScanKind(StrEnum):
    NEW_SCAN = "new_scan"
    REFRESH = "refresh"
    BACKFILL = "backfill"


class CoverageWindowView(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    id: UUID
    starts_at: datetime
    ends_at: datetime
    status: CoverageWindowStatus
    stop_reason: str | None
    page_count: int = Field(ge=0)

    @field_validator("starts_at", "ends_at")
    @classmethod
    def require_utc(cls, value: datetime) -> datetime:
        if value.utcoffset() != timedelta(0):
            raise ValueError("coverage window bounds must be UTC")
        return value


class DueAdmissionState(StrEnum):
    PENDING = "pending"
    ACCEPTED = "accepted"
    SKIPPED = "skipped"
    MISSED = "missed"


class DueSkipReason(StrEnum):
    QUIET = "quiet"
    DISABLED = "disabled"
    RATE_LIMITED = "rate_limited"
    BUDGET = "budget"


class CollectionDueWindowInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    owner_id: UUID
    schedule_key: UUID
    topic_id: UUID | None
    source_key: str = Field(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_-]{0,63}$")
    capability: SourceCapability
    due_at: datetime
    window_start: datetime
    window_end: datetime
    connection_version: int | None = Field(default=None, ge=1)
    policy_snapshot: SourceExecutionPolicy | None = None

    @field_validator("due_at", "window_start", "window_end")
    @classmethod
    def require_utc(cls, value: datetime) -> datetime:
        if value.utcoffset() != timedelta(0):
            raise ValueError("due window timestamps must be UTC")
        return value

    @model_validator(mode="after")
    def validate_bounds(self) -> CollectionDueWindowInput:
        if self.window_start >= self.window_end or self.window_end > self.due_at:
            raise ValueError("due window must be a half-open range ending by due_at")
        return self


class CollectionDueWindowView(CollectionDueWindowInput):
    id: UUID
    admission_state: DueAdmissionState
    reason: str | None
    operation_id: UUID | None
    job_id: UUID | None
    recorded_at: datetime


class CollectionExecutionFactView(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    due: CollectionDueWindowView
    job_status: JobStatus | None
    requests_sent: int | None = Field(ge=0)
    request_attempt_count: int | None = Field(ge=0)
    charged_request_count: int | None = Field(ge=0)
    request_budget_reconciled: bool | None
    page_count: int | None = Field(ge=0)
    observed_count: int | None = Field(ge=0)
    coverage_status: CoverageWindowStatus | None
    stop_reason: str | None
    has_gap: bool


class AnalysisJobFactView(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    id: UUID
    status: JobStatus
    scope: dict[str, JobScopeValue]
    last_error_code: str | None


class JobControlStatus(StrEnum):
    QUEUED = "queued"
    RUNNING = "running"
    CANCELLING = "cancelling"
    SUCCEEDED = "succeeded"
    PARTIALLY_SUCCEEDED = "partially_succeeded"
    FAILED = "failed"
    CANCELLED = "cancelled"


class JobFailureCategory(StrEnum):
    TRANSIENT = "transient"
    RATE_LIMITED = "rate_limited"
    AUTHENTICATION_REQUIRED = "authentication_required"
    PERMISSION_DENIED = "permission_denied"
    INVALID_RESPONSE = "invalid_response"
    PARSE_ERROR = "parse_error"
    INVALID_INPUT = "invalid_input"
    CONFIGURATION_UNAVAILABLE = "configuration_unavailable"


class JobDelayReason(StrEnum):
    INTERNAL_QUEUE = "internal_queue"
    RATE_LIMITED = "rate_limited"
    BUDGET_EXHAUSTED = "budget_exhausted"
    TRANSIENT_FAILURE = "transient_failure"
    MANUAL_RETRY = "manual_retry"
    OTHER = "other"


class CollectionJobKind(StrEnum):
    MONITOR_COLLECT = "monitor.collect"
    WEBPAGE_COLLECT = "webpage.collect"


class OperationalTaskStatus(StrEnum):
    QUEUED = "queued"
    DELAYED = "delayed"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    PARTIALLY_SUCCEEDED = "partially_succeeded"
    FAILED = "failed"
    CANCELLED = "cancelled"


class JobStage(StrEnum):
    REQUEST = "request"
    PARSE = "parse"
    SAVE = "save"
    ANALYSIS = "analysis"


class JobStageOutcome(StrEnum):
    STARTED = "started"
    SUCCEEDED = "succeeded"
    PARTIALLY_SUCCEEDED = "partially_succeeded"
    FAILED = "failed"
    DELAYED = "delayed"
    CANCELLED = "cancelled"


class SourceTimePrecision(StrEnum):
    SECOND = "second"
    MINUTE = "minute"
    DAY = "day"
    UNKNOWN = "unknown"


class SourceTimeStatus(StrEnum):
    VALID = "valid"
    UNKNOWN = "unknown"
    MISSING_TIMEZONE = "missing_timezone"
    FUTURE_SKEW = "future_skew"


class CostClass(StrEnum):
    LOCAL = "local"
    ZERO_PRICE = "zero_price"
    FREE_CREDIT = "free_credit"
    PAID = "paid"
    UNKNOWN = "unknown"


class UsageKind(StrEnum):
    NETWORK_REQUEST = "network_request"
    COLLECTOR_CALL = "collector_call"
    ANALYSIS_ATTEMPT = "analysis_attempt"


class UsageOutcome(StrEnum):
    STARTED = "started"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    FILTERED = "filtered"
    EMPTY = "empty"


class BudgetMetric(StrEnum):
    NETWORK_REQUEST = "network_request"
    COLLECTOR_CALL = "collector_call"
    ANALYSIS_ATTEMPT = "analysis_attempt"
    CONCURRENCY_SLOT = "concurrency_slot"
    X_API_USD_MICROS = "x_api_usd_micros"


class XApiPostReadCost(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    max_posts: int = Field(gt=0, le=100)
    unit_price_usd_micros: int = Field(gt=0, le=_MAX_BUDGET_UNITS)

    @model_validator(mode="after")
    def validate_reservation_size(self) -> XApiPostReadCost:
        if self.max_posts * self.unit_price_usd_micros > _MAX_BUDGET_UNITS:
            raise ValueError("x api reservation exceeds BIGINT")
        return self

    @property
    def reservation_units(self) -> int:
        return self.max_posts * self.unit_price_usd_micros

    def settlement_units(self, returned_posts: int | None) -> int:
        if returned_posts is None:
            return self.reservation_units
        if type(returned_posts) is not int or not 0 <= returned_posts <= self.max_posts:
            raise ValueError("returned_posts must be within the reserved page")
        return returned_posts * self.unit_price_usd_micros


class XApiUserReadCost(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    resource_kind: Literal["user"] = "user"
    unit_price_usd_micros: int = Field(gt=0, le=_MAX_BUDGET_UNITS)

    @property
    def reservation_units(self) -> int:
        return self.unit_price_usd_micros

    def settlement_units(self, returned_users: int | None) -> int:
        if returned_users is None:
            return self.reservation_units
        if type(returned_users) is not int or returned_users not in {0, 1}:
            raise ValueError("returned_users must be zero or one")
        return returned_users * self.unit_price_usd_micros


XApiReadCostQuote = XApiPostReadCost | XApiUserReadCost


class BudgetScopeKind(StrEnum):
    GLOBAL = "global"
    SOURCE = "source"
    CONNECTION = "connection"
    JOB = "job"


class BudgetDecisionStatus(StrEnum):
    RESERVED = "reserved"
    DELAYED = "delayed"


class BudgetResumeCondition(StrEnum):
    NEXT_WINDOW = "next_window"
    CAPACITY_RELEASE = "capacity_release"


class BudgetReservationStatus(StrEnum):
    RESERVED = "reserved"
    SETTLED = "settled"


class BudgetContext(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    source_ref: str | None = Field(default=None, max_length=128)
    connection_ref: str | None = Field(default=None, max_length=128)
    job_ref: str | None = Field(default=None, max_length=128)

    @field_validator("source_ref", "connection_ref", "job_ref")
    @classmethod
    def validate_reference(cls, value: str | None) -> str | None:
        if value is not None and _STABLE_REFERENCE_PATTERN.fullmatch(value) is None:
            raise ValueError("budget references must be stable lowercase identifiers")
        return value


class BudgetPolicyInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    budget_key: str = Field(
        min_length=1,
        max_length=128,
        pattern=r"^[a-z][a-z0-9_.:-]{0,127}$",
    )
    metric: BudgetMetric
    scope_kind: BudgetScopeKind
    scope_reference: str | None = Field(default=None, max_length=128)
    limit_units: int = Field(gt=0, le=_MAX_BUDGET_UNITS)
    window_seconds: int = Field(gt=0)
    window_anchor_at: datetime
    enabled: bool

    @model_validator(mode="after")
    def validate_scope_and_window(self) -> BudgetPolicyInput:
        if self.scope_kind is BudgetScopeKind.GLOBAL:
            if self.scope_reference is not None:
                raise ValueError("global scope cannot have scope_reference")
        elif (
            self.scope_reference is None
            or _STABLE_REFERENCE_PATTERN.fullmatch(self.scope_reference) is None
        ):
            raise ValueError("non-global scope_reference must be a stable lowercase identifier")
        if self.window_anchor_at.tzinfo is None:
            raise ValueError("window_anchor_at must be timezone-aware")
        if (
            self.metric is BudgetMetric.X_API_USD_MICROS
            and self.scope_kind is BudgetScopeKind.SOURCE
            and self.scope_reference != "x"
        ):
            raise ValueError("x api spend policy requires x source")
        return self


class BudgetPolicyView(BudgetPolicyInput):
    id: UUID
    owner_id: UUID
    policy_version: int = Field(ge=1)
    created_at: datetime
    updated_at: datetime


class BudgetWindowUsageView(BaseModel):
    model_config = ConfigDict(frozen=True)

    budget_policy_id: UUID
    budget_key: str
    metric: BudgetMetric
    scope_kind: BudgetScopeKind
    scope_reference: str | None
    limit_units: int = Field(gt=0)
    window_seconds: int = Field(gt=0)
    window_anchor_at: datetime
    enabled: bool
    policy_version: int = Field(ge=1)
    window_start: datetime | None
    window_end: datetime | None
    used_units: int = Field(ge=0)
    reserved_units: int = Field(ge=0)
    remaining_units: int | None = Field(ge=0)
    next_window_at: datetime | None


class BudgetReservationInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    reservation_id: UUID
    operation_id: UUID
    metric: BudgetMetric
    requested_units: int = Field(gt=0, le=_MAX_BUDGET_UNITS)
    context: BudgetContext
    cost_quote: XApiReadCostQuote | None = None

    @model_validator(mode="after")
    def validate_x_source(self) -> BudgetReservationInput:
        if self.metric is BudgetMetric.X_API_USD_MICROS:
            if self.context.source_ref != "x":
                raise ValueError("x api spend budget requires x source")
            if (
                not isinstance(self.cost_quote, (XApiPostReadCost, XApiUserReadCost))
                or self.requested_units != self.cost_quote.reservation_units
            ):
                raise ValueError("x api spend budget requires a matching cost quote")
        elif self.cost_quote is not None:
            raise ValueError("cost quote requires x api spend budget")
        return self


class BudgetReservationDecision(BaseModel):
    model_config = ConfigDict(frozen=True)

    status: BudgetDecisionStatus
    reservation_id: UUID
    operation_id: UUID
    metric: BudgetMetric
    requested_units: int = Field(gt=0)
    remaining_units: int = Field(ge=0)
    limiting_budget_keys: tuple[str, ...]
    resume_condition: BudgetResumeCondition | None
    retry_at: datetime | None


class BudgetSettlementView(BaseModel):
    model_config = ConfigDict(frozen=True)

    reservation_id: UUID
    operation_id: UUID
    metric: BudgetMetric
    requested_units: int = Field(gt=0)
    actual_units: int = Field(ge=0)
    released_units: int = Field(ge=0)
    policy_count: int = Field(gt=0)
    settled_at: datetime


class ComponentPolicyInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    component_key: str = Field(
        min_length=1,
        max_length=128,
        pattern=r"^[a-z][a-z0-9_.:-]{0,127}$",
    )
    component_version: str = Field(min_length=1, max_length=128)
    cost_class: CostClass
    enabled_for_core: bool
    terms_reference: str = Field(min_length=1, max_length=512)
    reviewed_at: datetime

    @model_validator(mode="after")
    def validate_core_eligibility(self) -> ComponentPolicyInput:
        if self.enabled_for_core and self.cost_class not in {
            CostClass.LOCAL,
            CostClass.ZERO_PRICE,
        }:
            raise ValueError("core components must be local or zero_price")
        if self.reviewed_at.tzinfo is None:
            raise ValueError("reviewed_at must be timezone-aware")
        return self


class ComponentPolicyView(ComponentPolicyInput):
    id: UUID
    owner_id: UUID
    policy_version: int = Field(ge=1)
    created_at: datetime
    updated_at: datetime


class UsageAttemptInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    attempt_id: UUID
    operation_id: UUID
    component_key: str = Field(
        min_length=1,
        max_length=128,
        pattern=r"^[a-z][a-z0-9_.:-]{0,127}$",
    )
    usage_kind: UsageKind
    stage: str = Field(
        min_length=1,
        max_length=128,
        pattern=r"^[a-z][a-z0-9_.:-]{0,127}$",
    )
    started_at: datetime

    @field_validator("started_at")
    @classmethod
    def validate_started_at(cls, value: datetime) -> datetime:
        if value.tzinfo is None:
            raise ValueError("started_at must be timezone-aware")
        return value


class UsageAttemptView(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: UUID
    owner_id: UUID
    attempt_id: UUID
    operation_id: UUID
    component_policy_id: UUID
    component_version: str
    usage_kind: UsageKind
    stage: str
    outcome: UsageOutcome
    started_at: datetime
    finished_at: datetime | None


class UsageSummaryView(BaseModel):
    model_config = ConfigDict(frozen=True)

    owner_id: UUID
    operation_id: UUID
    total_attempts: int = Field(ge=0)
    started_attempts: int = Field(ge=0)
    succeeded_attempts: int = Field(ge=0)
    failed_attempts: int = Field(ge=0)
    filtered_attempts: int = Field(ge=0)
    empty_attempts: int = Field(ge=0)


class JobObservationContext(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    configuration_ref: str = Field(
        min_length=1,
        max_length=128,
        pattern=r"^[a-z0-9][a-z0-9_.:-]{0,127}$",
    )
    configuration_version: int = Field(ge=1)
    source_key: str | None = Field(
        default=None,
        min_length=1,
        max_length=64,
        pattern=r"^[a-z][a-z0-9_-]{0,63}$",
    )
    source_capability: SourceCapability | None = None

    @model_validator(mode="after")
    def validate_source_pair(self) -> JobObservationContext:
        if (self.source_key is None) != (self.source_capability is None):
            raise ValueError("source_key and source_capability must be provided together")
        return self


class StageAttemptInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    attempt_id: UUID
    stage: JobStage
    attempt_sequence: int = Field(ge=1)
    started_at: datetime

    @field_validator("started_at")
    @classmethod
    def validate_started_at(cls, value: datetime) -> datetime:
        if value.tzinfo is None:
            raise ValueError("started_at must be timezone-aware")
        return value


class StageAttemptView(StageAttemptInput):
    owner_id: UUID
    job_id: UUID
    outcome: JobStageOutcome
    finished_at: datetime | None


class OperationalTaskRecord(BaseModel):
    model_config = ConfigDict(frozen=True)

    job_id: UUID
    operation_id: UUID
    kind: str
    observation: JobObservationContext
    status: OperationalTaskStatus
    scheduled_for_at: datetime | None
    created_at: datetime
    started_at: datetime | None
    completed_at: datetime | None
    next_run_at: datetime | None
    execution_attempts: int = Field(ge=0)
    stage_attempts: int = Field(ge=0)


class OperationAttemptCount(BaseModel):
    model_config = ConfigDict(frozen=True)

    operation_id: UUID
    attempts: int = Field(ge=0)


def _validate_operational_task_counts(counts: dict[OperationalTaskStatus, int], total: int) -> None:
    if set(counts) != set(OperationalTaskStatus):
        raise ValueError("task_counts must include every operational status")
    if any(value < 0 for value in counts.values()):
        raise ValueError("task counts cannot be negative")
    if sum(counts.values()) != total:
        raise ValueError("task counts must reconcile to total_tasks")


class OperationalSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    total_tasks: int = Field(ge=0)
    task_counts: dict[OperationalTaskStatus, int]
    execution_attempts: int = Field(ge=0)
    stage_attempts: int = Field(ge=0)
    resource_attempts: int = Field(ge=0)

    def count(self, status: OperationalTaskStatus) -> int:
        return self.task_counts.get(status, 0)

    @model_validator(mode="after")
    def validate_task_total(self) -> OperationalSummary:
        _validate_operational_task_counts(self.task_counts, self.total_tasks)
        return self


class SourceCapabilityTaskSummary(BaseModel):
    model_config = ConfigDict(frozen=True)

    source_key: str = Field(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_-]{0,63}$")
    source_capability: SourceCapability
    total_tasks: int = Field(gt=0)
    task_counts: dict[OperationalTaskStatus, int]

    @model_validator(mode="after")
    def validate_task_total(self) -> SourceCapabilityTaskSummary:
        _validate_operational_task_counts(self.task_counts, self.total_tasks)
        return self


class OperationalSnapshot(BaseModel):
    model_config = ConfigDict(frozen=True)

    owner_id: UUID
    window_start: datetime
    window_end: datetime
    tasks: tuple[OperationalTaskRecord, ...]
    operations: tuple[OperationAttemptCount, ...]
    summary: OperationalSummary
    capabilities: tuple[SourceCapabilityTaskSummary, ...]


class JobReliabilityRecord(BaseModel):
    model_config = ConfigDict(frozen=True)

    job_id: UUID
    operation_id: UUID
    kind: str
    outcome: JobReliabilityOutcome
    logical_start_at: datetime
    sla_deadline_at: datetime
    completed_at: datetime | None
    elapsed_us: int | None = Field(default=None, ge=0)
    failed_source_evidence_count: int = Field(ge=0)
    on_time: bool

    @model_validator(mode="after")
    def validate_evidence_and_times(self) -> JobReliabilityRecord:
        if self.logical_start_at.utcoffset() is None:
            raise ValueError("logical_start_at must be timezone-aware")
        if self.sla_deadline_at.utcoffset() is None:
            raise ValueError("sla_deadline_at must be timezone-aware")
        if self.sla_deadline_at <= self.logical_start_at:
            raise ValueError("sla_deadline_at must follow logical_start_at")
        if self.completed_at is not None and self.completed_at.utcoffset() is None:
            raise ValueError("completed_at must be timezone-aware")
        if self.completed_at is None and self.elapsed_us is not None:
            raise ValueError("non-terminal observation cannot have terminal elapsed time")
        if self.completed_at is None and self.outcome is not JobReliabilityOutcome.IN_PROGRESS:
            raise ValueError("terminal outcome requires completed_at")
        if self.completed_at is not None and self.outcome is JobReliabilityOutcome.IN_PROGRESS:
            raise ValueError("in-progress outcome cannot have completed_at")
        if (
            self.outcome is JobReliabilityOutcome.SOURCE_FAILURE
            and self.failed_source_evidence_count == 0
        ):
            raise ValueError("source failure requires persisted source evidence")
        if (
            self.outcome is JobReliabilityOutcome.UNATTRIBUTED_FAILURE
            and self.failed_source_evidence_count != 0
        ):
            raise ValueError("unattributed failure cannot have matching source evidence")
        if self.on_time and (
            self.completed_at is None
            or self.elapsed_us is None
            or self.outcome
            not in {JobReliabilityOutcome.SUCCEEDED, JobReliabilityOutcome.SOURCE_FAILURE}
        ):
            raise ValueError("only a completed success or persisted source failure can be on time")
        return self


def _validate_reliability_counts(counts: dict[JobReliabilityOutcome, int], total: int) -> None:
    if set(counts) != set(JobReliabilityOutcome):
        raise ValueError("outcome_counts must include every reliability outcome")
    if any(value < 0 for value in counts.values()):
        raise ValueError("outcome counts cannot be negative")
    if sum(counts.values()) != total:
        raise ValueError("outcome counts must reconcile to total_jobs")


class JobReliabilitySnapshot(BaseModel):
    model_config = ConfigDict(frozen=True)

    window_start: datetime
    window_end: datetime
    sla_seconds: int = Field(gt=0)
    total_jobs: int = Field(ge=0)
    on_time_jobs: int = Field(ge=0)
    outcome_counts: dict[JobReliabilityOutcome, int]
    records: tuple[JobReliabilityRecord, ...]

    @model_validator(mode="after")
    def validate_snapshot(self) -> JobReliabilitySnapshot:
        if self.window_start.utcoffset() is None or self.window_end.utcoffset() is None:
            raise ValueError("reliability window must be timezone-aware")
        if self.window_end <= self.window_start:
            raise ValueError("reliability window end must follow start")
        if len(self.records) != self.total_jobs:
            raise ValueError("reliability records must reconcile to total_jobs")
        if self.on_time_jobs > self.total_jobs:
            raise ValueError("on_time_jobs cannot exceed total_jobs")
        _validate_reliability_counts(self.outcome_counts, self.total_jobs)
        record_counts = {
            outcome: sum(record.outcome is outcome for record in self.records)
            for outcome in JobReliabilityOutcome
        }
        if self.outcome_counts != record_counts:
            raise ValueError("outcome_counts must match reliability records")
        expected_on_time_jobs = 0
        for record in self.records:
            if record.kind != "webpage.collect":
                raise ValueError("webpage.collect is the only task kind with a frozen SLA")
            if record.sla_deadline_at != record.logical_start_at + timedelta(
                seconds=self.sla_seconds
            ):
                raise ValueError("sla_deadline_at must match the configured SLA")
            if not self.window_start <= record.sla_deadline_at < self.window_end:
                raise ValueError("record SLA deadline must be inside the observation window")
            if record.on_time:
                if record.elapsed_us is None:
                    raise ValueError("on-time record requires terminal duration")
                if record.elapsed_us > self.sla_seconds * 1_000_000:
                    raise ValueError("on-time record cannot exceed its SLA")
            expected_on_time_jobs += record.on_time
        if self.on_time_jobs != expected_on_time_jobs:
            raise ValueError("on_time_jobs must match reliability records")
        return self

    @property
    def on_time_rate(self) -> float | None:
        if self.total_jobs == 0:
            return None
        return self.on_time_jobs / self.total_jobs


class FreshnessTimelineInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=True)

    scheduled_for_at: datetime | None = None
    accepted_at: datetime
    started_at: datetime | None = None
    request_started_at: datetime | None = None
    source_published_at: datetime | None = None
    source_time_precision: SourceTimePrecision = SourceTimePrecision.UNKNOWN
    source_observed_at: datetime | None = None
    persisted_at: datetime | None = None
    queryable_at: datetime | None = None

    @model_validator(mode="after")
    def validate_time_chain(self) -> FreshnessTimelineInput:
        internal_times = (
            self.scheduled_for_at,
            self.accepted_at,
            self.started_at,
            self.request_started_at,
            self.source_observed_at,
            self.persisted_at,
            self.queryable_at,
        )
        if any(value is not None and value.utcoffset() is None for value in internal_times):
            raise ValueError("internal freshness timestamps must be timezone-aware")
        if self.started_at is not None:
            if self.started_at < self.accepted_at:
                raise ValueError("started_at cannot precede accepted_at")
            if self.scheduled_for_at is not None and self.started_at < self.scheduled_for_at:
                raise ValueError("started_at cannot precede scheduled_for_at")
        self._require_ordered(
            self.started_at,
            self.request_started_at,
            "request_started_at",
        )
        self._require_ordered(
            self.request_started_at,
            self.source_observed_at,
            "source_observed_at",
        )
        self._require_ordered(
            self.source_observed_at,
            self.persisted_at,
            "persisted_at",
        )
        self._require_ordered(
            self.persisted_at,
            self.queryable_at,
            "queryable_at",
        )
        if (
            self.source_published_at is None
            and self.source_time_precision is not SourceTimePrecision.UNKNOWN
        ):
            raise ValueError("source precision requires a published timestamp")
        return self

    @staticmethod
    def _require_ordered(
        previous: datetime | None,
        current: datetime | None,
        field_name: str,
    ) -> None:
        if current is None:
            return
        if previous is None:
            raise ValueError(f"{field_name} requires its previous stage")
        if current < previous:
            raise ValueError(f"{field_name} cannot precede its previous stage")


class FreshnessTimelineView(FreshnessTimelineInput):
    source_time_status: SourceTimeStatus
    schedule_wait_us: int | None = Field(default=None, ge=0)
    queue_wait_us: int | None = Field(default=None, ge=0)
    internal_prepare_us: int | None = Field(default=None, ge=0)
    source_wait_us: int | None = Field(default=None, ge=0)
    processing_us: int | None = Field(default=None, ge=0)
    visibility_us: int | None = Field(default=None, ge=0)
    end_to_end_us: int | None = Field(default=None, ge=0)
    publication_to_observation_us: int | None = Field(default=None, ge=0)


class JobAcceptanceInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_id: UUID
    kind: str = Field(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_.-]{0,63}$")
    observation: JobObservationContext
    scheduled_for_at: datetime | None = None
    scope: dict[str, JobScopeValue]

    @field_validator("scheduled_for_at")
    @classmethod
    def validate_scheduled_for_at(cls, value: datetime | None) -> datetime | None:
        if value is not None and value.utcoffset() is None:
            raise ValueError("scheduled_for_at must be timezone-aware")
        return value

    @field_validator("scope")
    @classmethod
    def validate_scope(
        cls,
        value: dict[str, JobScopeValue],
    ) -> dict[str, JobScopeValue]:
        if len(value) > _MAX_SCOPE_ITEMS:
            raise ValueError(f"scope cannot contain more than {_MAX_SCOPE_ITEMS} items")
        if any(_SCOPE_KEY_PATTERN.fullmatch(key) is None for key in value):
            raise ValueError("scope keys must be stable lowercase identifiers")
        return value


class WebPageCollectionJobInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_id: UUID
    kind: Literal[CollectionJobKind.WEBPAGE_COLLECT] = Field(
        json_schema_extra={"enum": [CollectionJobKind.WEBPAGE_COLLECT.value]}
    )
    url: str = Field(min_length=1, max_length=2048)

    @field_validator("url")
    @classmethod
    def validate_url(cls, value: str) -> str:
        return WebPageRequest(url=value).url


class JobProgressView(BaseModel):
    model_config = ConfigDict(frozen=True)

    stage: JobStage | None
    requests_sent: int = Field(ge=0)
    items_saved: int = Field(ge=0)
    updated_at: datetime | None


class JobCancellationView(BaseModel):
    model_config = ConfigDict(frozen=True)

    requested_at: datetime
    deadline_at: datetime | None
    timed_out: bool


class JobFailureView(BaseModel):
    model_config = ConfigDict(frozen=True)

    error_code: str
    category: JobFailureCategory
    occurred_at: datetime
    next_action: str
    manual_retry_allowed: bool


class JobContinuousFailureIssueView(BaseModel):
    model_config = ConfigDict(frozen=True)

    source_key: str = Field(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_-]{0,63}$")
    source_capability: SourceCapability
    configuration_ref: str = Field(
        min_length=1,
        max_length=128,
        pattern=r"^[a-z0-9][a-z0-9_.:-]{0,127}$",
    )
    configuration_version: int = Field(ge=1)
    latest_failed_job_id: UUID
    failure: JobFailureView
    consecutive_failure_threshold: Literal[3]


class SourceFreshnessView(BaseModel):
    model_config = ConfigDict(frozen=True)

    last_attempt_at: datetime | None
    last_success_at: datetime | None
    delay_reason: JobDelayReason | None
    delay_since_at: datetime | None
    delay_duration_us: int | None = Field(ge=0)


class JobStatusView(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: UUID
    operation_id: UUID
    kind: str
    observation: JobObservationContext
    status: JobControlStatus
    progress: JobProgressView
    cancellation: JobCancellationView | None
    failure: JobFailureView | None
    result_content_id: UUID | None
    retry_count: int = Field(ge=0)
    next_run_at: datetime | None
    scheduled_for_at: datetime | None
    started_at: datetime | None
    completed_at: datetime | None
    created_at: datetime
    source_freshness: SourceFreshnessView | None = None
    coverage_windows: tuple[CoverageWindowView, ...] = ()


class JobHistoryItemView(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: UUID
    kind: str
    source_key: str | None
    source_capability: SourceCapability | None
    status: JobControlStatus
    requests_sent: int = Field(ge=0)
    items_saved: int = Field(ge=0)
    created_at: datetime
    started_at: datetime | None
    completed_at: datetime | None
    next_run_at: datetime | None


class JobView(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: UUID
    owner_id: UUID
    operation_id: UUID
    kind: str
    observation: JobObservationContext
    status: JobStatus
    scheduled_for_at: datetime | None
    started_at: datetime | None
    completed_at: datetime | None
    created_at: datetime


class JobAcceptedMessage(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: Literal[2]
    message_id: UUID
    event_type: Literal["job.accepted.v2"]
    job_id: UUID
    owner_id: UUID
    operation_id: UUID
    kind: str = Field(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_.-]{0,63}$")
    configuration_ref: str = Field(
        min_length=1,
        max_length=128,
        pattern=r"^[a-z0-9][a-z0-9_.:-]{0,127}$",
    )
    configuration_version: int = Field(ge=1)
    source_key: str | None = Field(
        default=None,
        min_length=1,
        max_length=64,
        pattern=r"^[a-z][a-z0-9_-]{0,63}$",
    )
    source_capability: SourceCapability | None = None

    @model_validator(mode="after")
    def validate_source_pair(self) -> JobAcceptedMessage:
        if (self.source_key is None) != (self.source_capability is None):
            raise ValueError("source_key and source_capability must be provided together")
        return self


class JobRetryScheduledMessage(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: Literal[1]
    message_id: UUID
    event_type: Literal["job.retry_scheduled.v1"]
    job_id: UUID
    owner_id: UUID
    operation_id: UUID
    kind: str = Field(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_.-]{0,63}$")
    configuration_ref: str = Field(
        min_length=1,
        max_length=128,
        pattern=r"^[a-z0-9][a-z0-9_.:-]{0,127}$",
    )
    configuration_version: int = Field(ge=1)
    source_key: str | None = Field(
        default=None,
        min_length=1,
        max_length=64,
        pattern=r"^[a-z][a-z0-9_-]{0,63}$",
    )
    source_capability: SourceCapability | None = None
    dispatch_sequence: int = Field(ge=2)
    retry_count: int = Field(ge=1)
    retry_at: datetime
    last_error_code: str = Field(
        min_length=1,
        max_length=128,
        pattern=r"^[a-z][a-z0-9_.:-]{0,127}$",
    )

    @model_validator(mode="after")
    def validate_retry(self) -> JobRetryScheduledMessage:
        if (self.source_key is None) != (self.source_capability is None):
            raise ValueError("source_key and source_capability must be provided together")
        if self.retry_at.tzinfo is None:
            raise ValueError("retry_at must be timezone-aware")
        return self


type JobMessage = JobAcceptedMessage | JobRetryScheduledMessage
