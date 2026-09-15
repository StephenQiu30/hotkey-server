from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, Field

from core.schemas import Input
from sources.schemas import SourceName


class EventInput(Input):
    title: str = Field(min_length=1, max_length=200)
    summary: str = Field(default="", max_length=2000)


class EventMemberInput(Input):
    content_id: UUID


class EventMemberView(BaseModel):
    content_id: UUID
    source: SourceName
    kind: Literal["post", "comment", "reply"]
    external_id: str
    canonical_url: str | None
    added_at: datetime


class EventView(BaseModel):
    id: UUID
    title: str
    summary: str
    status: Literal["active", "archived"]
    current_revision: int
    created_at: datetime
    updated_at: datetime
    members: list[EventMemberView]


class EventPage(BaseModel):
    items: list[EventView]
    next_cursor: UUID | None


class EventSnapshot(BaseModel):
    title: str
    summary: str
    status: Literal["active", "archived"]
    member_content_ids: list[UUID]


class EventRevisionView(BaseModel):
    revision: int
    change_type: Literal["create", "add_member", "remove_member"]
    snapshot: EventSnapshot
    created_at: datetime
