from __future__ import annotations

from datetime import datetime
from typing import Any

from pydantic import BaseModel, ConfigDict, Field


class SettingUpsert(BaseModel):
    value: dict[str, Any] = Field(default_factory=dict)
    description: str | None = None


class SettingRead(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    key: str
    value: dict[str, Any]
    description: str | None
    created_at: datetime
    updated_at: datetime
