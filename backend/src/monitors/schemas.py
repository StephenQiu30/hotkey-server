from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator

from core.schemas import Input
from sources.schemas import QuerySpec, SourceName

MonitorState = Literal["draft", "active", "paused"]


class ScheduleSpec(BaseModel):
    interval_minutes: int = Field(default=60, ge=15, le=1440)
    retention_days: int = Field(default=7, ge=1, le=365)


class BudgetSpec(BaseModel):
    daily_requests: int = Field(default=120, ge=1, le=1000)
    content_purchase_cost: Literal[0] = 0


class MonitorInput(Input):
    title: str = Field(min_length=1, max_length=100)
    query_spec: QuerySpec
    source_ids: list[SourceName] = Field(min_length=1, max_length=6)
    schedule: ScheduleSpec = Field(default_factory=ScheduleSpec)
    budget: BudgetSpec = Field(default_factory=BudgetSpec)

    @field_validator("title")
    @classmethod
    def title_clean(cls, value: str) -> str:
        cleaned = value.strip()
        if not cleaned:
            raise ValueError("title cannot be blank")
        return cleaned

    @field_validator("source_ids")
    @classmethod
    def sources_unique(cls, values: list[SourceName]) -> list[SourceName]:
        return list(dict.fromkeys(values))


class MonitorUpdate(MonitorInput):
    expected_version: int = Field(ge=1)


class MonitorStateChange(Input):
    expected_version: int = Field(ge=1)


class MonitorView(BaseModel):
    id: UUID
    title: str
    state: MonitorState
    current_version: int
    query_spec: QuerySpec
    source_ids: list[SourceName]
    schedule: ScheduleSpec
    budget: BudgetSpec
    created_at: datetime
    updated_at: datetime


class ActiveMonitorConfiguration(BaseModel):
    model_config = ConfigDict(frozen=True)
    monitor_id: UUID
    monitor_version_id: UUID
    version: int = Field(ge=1)
    query_spec: QuerySpec
    source_ids: list[SourceName]
    schedule: ScheduleSpec
    budget: BudgetSpec


class MonitorPage(BaseModel):
    items: list[MonitorView]
    next_cursor: UUID | None
