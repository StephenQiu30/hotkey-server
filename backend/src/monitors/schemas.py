from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator

from core.schemas import Input
from sources.schemas import SourceName


class MonitorInput(Input):
    title: str = Field(min_length=1, max_length=100)
    keywords: list[str] = Field(min_length=1, max_length=20)
    sources: list[SourceName] = Field(min_length=1, max_length=6)

    @field_validator("keywords")
    @classmethod
    def keywords_valid(cls, values: list[str]) -> list[str]:
        cleaned = [v.strip() for v in values]
        if any(not v or len(v) > 100 for v in cleaned):
            raise ValueError("keywords must contain 1 to 100 characters")
        return list(dict.fromkeys(cleaned))

    @field_validator("sources")
    @classmethod
    def sources_unique(cls, values: list[SourceName]) -> list[SourceName]:
        return list(dict.fromkeys(values))


class MonitorUpdate(MonitorInput):
    version: int = Field(ge=1)


class MonitorView(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: UUID
    title: str
    keywords: list[str]
    sources: list[str]
    version: int
    created_at: datetime
    status: Literal["draft"] = "draft"


class MonitorPage(BaseModel):
    items: list[MonitorView]
    next_cursor: UUID | None
