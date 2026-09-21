from __future__ import annotations

from datetime import datetime
from enum import StrEnum
from typing import Annotated
from uuid import UUID

from pydantic import ConfigDict, Field

from core.schemas import InputModel, OutputModel

type KeywordInput = Annotated[str, Field(min_length=1, max_length=100)]


class MonitorTopicStatus(StrEnum):
    PAUSED = "paused"
    ACTIVE = "active"
    ARCHIVED = "archived"


class MonitorTopicReadinessStatus(StrEnum):
    PENDING_SOURCE_SELECTION = "pending_source_selection"
    PENDING_SOURCE_READINESS = "pending_source_readiness"
    READY = "ready"


class MonitorRuleSetInput(InputModel):
    match_any: list[KeywordInput] = Field(max_length=50)
    match_all: list[KeywordInput] = Field(max_length=50)
    exclude: list[KeywordInput] = Field(max_length=50)


class MonitorTopicCreateInput(MonitorRuleSetInput):
    name: str = Field(min_length=1, max_length=80)


class MonitorTopicUpdateInput(MonitorTopicCreateInput):
    expected_version: int = Field(ge=1)


class MonitorRuleSetView(OutputModel):
    model_config = ConfigDict(frozen=True)

    match_any: list[str]
    match_all: list[str]
    exclude: list[str]


class MonitorTopicView(OutputModel):
    id: UUID
    name: str
    status: MonitorTopicStatus
    readiness_status: MonitorTopicReadinessStatus
    current_version: int = Field(ge=1)
    rules: MonitorRuleSetView
    created_at: datetime
    updated_at: datetime
