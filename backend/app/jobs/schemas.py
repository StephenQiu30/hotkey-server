from __future__ import annotations

import re
from datetime import datetime
from enum import StrEnum
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

type JobScopeValue = str | int | bool | None

_SCOPE_KEY_PATTERN = re.compile(r"^[a-z][a-z0-9_.:-]{0,63}$")
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
