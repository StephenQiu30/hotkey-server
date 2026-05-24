from __future__ import annotations

from datetime import datetime

from pydantic import BaseModel, ConfigDict


class CheckRunCreate(BaseModel):
    trigger_type: str = "manual"


class CheckRunRead(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    trigger_type: str
    started_at: datetime
    finished_at: datetime | None
    status: str
    success_count: int
    failure_count: int
    error_summary: str | None
    created_at: datetime
    updated_at: datetime
