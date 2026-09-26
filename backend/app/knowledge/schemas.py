from __future__ import annotations

from datetime import datetime
from enum import StrEnum
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class KnowledgeObjectType(StrEnum):
    DAILY = "daily"
    WEEKLY = "weekly"
    EVENT = "event"
    TOPIC = "topic"
    POST = "post"
    QA = "qa"


class DailyExportInput(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    owner_id: UUID
    report_id: UUID
    topic_id: UUID
    version: int = Field(ge=1)
    topic_name: str = Field(min_length=1)
    window_start: datetime
    generated_at: datetime
    generator: str
    body_markdown: str = Field(min_length=1)
    source_url: str = ""


class KnowledgeExportResult(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    relative_path: str
    content_sha256: str
    written: bool
