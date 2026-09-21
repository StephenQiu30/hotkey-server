from __future__ import annotations

import re
from datetime import datetime
from enum import StrEnum
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

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


class JobAcceptanceInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    operation_id: UUID
    kind: str = Field(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_.-]{0,63}$")
    scope: dict[str, JobScopeValue]

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


class JobView(BaseModel):
    model_config = ConfigDict(frozen=True)

    id: UUID
    owner_id: UUID
    operation_id: UUID
    kind: str
    status: JobStatus
    created_at: datetime


class JobAcceptedMessage(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    schema_version: Literal[1]
    message_id: UUID
    event_type: Literal["job.accepted.v1"]
    job_id: UUID
    owner_id: UUID
    operation_id: UUID
    kind: str = Field(min_length=1, max_length=64, pattern=r"^[a-z][a-z0-9_.-]{0,63}$")
