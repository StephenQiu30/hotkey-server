from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel


class NotificationView(BaseModel):
    id: UUID
    event_id: UUID
    change_id: UUID
    rule_version: int
    kind: Literal[
        "event_member_added",
        "event_member_removed",
        "event_merged_in",
        "event_merged_out",
        "event_split_in",
        "event_split_out",
    ]
    message: str
    created_at: datetime
    read_at: datetime | None


class NotificationPage(BaseModel):
    items: list[NotificationView]
    next_cursor: str | None
    unread_count: int
