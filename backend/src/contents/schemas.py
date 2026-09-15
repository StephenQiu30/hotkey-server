from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel

from sources.schemas import SourceName


class InboxItem(BaseModel):
    id: UUID
    source: SourceName
    provider_namespace: str
    external_id: str
    kind: Literal["post", "comment", "reply"]
    root_external_id: str
    parent_external_id: str | None
    relation_status: Literal["root", "unresolved", "resolved"]
    text: str
    version: int
    canonical_url: str | None
    published_at: datetime
    first_seen_at: datetime
    last_seen_at: datetime
    reply_count: int | None
    monitor_titles: list[str]


class InboxPage(BaseModel):
    items: list[InboxItem]
    next_cursor: str | None
