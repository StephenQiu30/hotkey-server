from __future__ import annotations

from datetime import date, datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict

ReportType = Literal["daily", "weekly"]


class ReportCreate(BaseModel):
    report_type: ReportType = "daily"
    period_start: date | None = None
    send: bool = False


class ReportRead(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    report_type: str
    period_start: datetime
    period_end: datetime
    status: str
    subject: str
    summary: str | None
    content: str
    hotspot_count: int
    sent_at: datetime | None
    created_at: datetime
    updated_at: datetime
