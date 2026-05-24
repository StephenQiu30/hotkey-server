from __future__ import annotations

from datetime import datetime

from pydantic import BaseModel, ConfigDict


class NotificationRead(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    hotspot_id: int | None
    report_id: int | None
    channel: str
    recipient: str | None
    status: str
    error_message: str | None
    sent_at: datetime | None
    created_at: datetime
    updated_at: datetime


class NotificationListResponse(BaseModel):
    items: list[NotificationRead]
    limit: int
    offset: int
