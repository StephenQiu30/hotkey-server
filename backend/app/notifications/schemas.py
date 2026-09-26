from __future__ import annotations

import re
from datetime import datetime
from enum import StrEnum
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator


class NotificationChannel(StrEnum):
    FEISHU = "feishu"
    EMAIL = "email"


class DeliveryStatus(StrEnum):
    PENDING = "pending"
    SENDING = "sending"
    SUCCEEDED = "succeeded"
    FAILED = "failed"
    UNKNOWN = "unknown"


class TargetInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    name: str = Field(min_length=1, max_length=80)
    channel: NotificationChannel
    recipients: tuple[str, ...] = Field(default=(), max_length=20)
    secret_env: str | None = Field(default=None, max_length=128)

    @field_validator("secret_env")
    @classmethod
    def validate_secret_env(cls, value: str | None) -> str | None:
        if value is not None and re.fullmatch(r"HOTKEY_[A-Z][A-Z0-9_]*", value) is None:
            raise ValueError("secret_env must name a HOTKEY_ environment variable")
        return value


class TargetView(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    id: UUID
    name: str
    channel: NotificationChannel
    recipients: tuple[str, ...]
    secret_env: str | None
    enabled: bool
    created_at: datetime
