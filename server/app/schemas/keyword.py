from __future__ import annotations

from datetime import datetime

from pydantic import BaseModel, ConfigDict


class KeywordBase(BaseModel):
    keyword: str
    query_template: str | None = None
    enabled: bool = True
    priority: int = 0


class KeywordCreate(KeywordBase):
    pass


class KeywordUpdate(BaseModel):
    keyword: str | None = None
    query_template: str | None = None
    enabled: bool | None = None
    priority: int | None = None


class KeywordRead(KeywordBase):
    model_config = ConfigDict(from_attributes=True)

    id: int
    created_at: datetime
    updated_at: datetime
