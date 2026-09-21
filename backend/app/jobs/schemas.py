from __future__ import annotations

import re
from datetime import datetime
from enum import StrEnum
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from sources.contracts import SourceCapability

type JobScopeValue = str | int | bool | None

_SCOPE_KEY_PATTERN = re.compile(r"^[a-z][a-z0-9_.:-]{0,63}$")
_STABLE_REFERENCE_PATTERN = re.compile(r"^[a-z0-9][a-z0-9_.:-]{0,127}$")
_MAX_SCOPE_ITEMS = 32


class JobStatus(StrEnum):
    QUEUED = "queued"
    RUNNING = "running"
    SUCCEEDED = "succeeded"
    PARTIALLY_SUCCEEDED = "partially_succeeded"
    FAILED = "failed"
    CANCELLED = "cancelled"


class JobControlStatus(StrEnum):
    QUEUED = "queued"
    RUNNING = "running"
    CANCELLING = "cancelling"
    SUCCEEDED = "succeeded"
    PARTIALLY_SUCCEEDED = "partially_succeeded"
    FAILED = "failed"
    CANCELLED = "cancelled"


class CollectionJobKind(StrEnum):
    MONITOR_COLLECT = "monitor.collect"


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
    ANALYSIS_ATTEMPT = "analysis_attempt"


class UsageOutcome(StrEnum):
    STARTED = "started"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    FILTERED = "filtered"
    EMPTY = "empty"


class BudgetMetric(StrEnum):
    NETWORK_REQUEST = "network_request"
    ANALYSIS_ATTEMPT = "analysis_attempt"
    CONCURRENCY_SLOT = "concurrency_slot"


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
    limit_units: int = Field(gt=0)
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
        return self


class BudgetPolicyView(BudgetPolicyInput):
    id: UUID
    owner_id: UUID
    policy_version: int = Field(ge=1)
    created_at: datetime
    updated_at: datetime


class BudgetReservationInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    reservation_id: UUID
    operation_id: UUID
    metric: BudgetMetric
    requested_units: int = Field(gt=0)
    context: BudgetContext


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


class CollectionJobObservationInput(JobObservationContext):
    model_config = ConfigDict(extra="forbid", frozen=True, strict=False)


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
        if set(self.task_counts) != set(OperationalTaskStatus):
            raise ValueError("task_counts must include every operational status")
        if any(value < 0 for value in self.task_counts.values()):
            raise ValueError("task counts cannot be negative")
        if sum(self.task_counts.values()) != self.total_tasks:
            raise ValueError("task counts must reconcile to total_tasks")
        return self


class OperationalSnapshot(BaseModel):
    model_config = ConfigDict(frozen=True)

    owner_id: UUID
    window_start: datetime
    window_end: datetime
    tasks: tuple[OperationalTaskRecord, ...]
    operations: tuple[OperationAttemptCount, ...]
    summary: OperationalSummary


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


class CollectionJobInput(JobAcceptanceInput):
    kind: CollectionJobKind
    observation: CollectionJobObservationInput

    def to_acceptance(self) -> JobAcceptanceInput:
        return JobAcceptanceInput.model_validate(self.model_dump())


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


class JobStatusView(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: UUID
    operation_id: UUID
    kind: str
    observation: JobObservationContext
    status: JobControlStatus
    progress: JobProgressView
    cancellation: JobCancellationView | None
    scheduled_for_at: datetime | None
    started_at: datetime | None
    completed_at: datetime | None
    created_at: datetime


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
