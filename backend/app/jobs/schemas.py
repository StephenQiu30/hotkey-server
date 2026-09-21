from __future__ import annotations

import re
from datetime import datetime
from enum import StrEnum
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator

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
