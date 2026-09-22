from __future__ import annotations

from datetime import datetime
from enum import StrEnum
from typing import Annotated, Literal
from uuid import UUID

from pydantic import ConfigDict, Field

from core.schemas import InputModel, OutputModel

type KeywordInput = Annotated[str, Field(min_length=1, max_length=100)]
type PreviewSampleInput = Annotated[str, Field(min_length=1, max_length=500)]


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


class MonitorTopicPreviewInput(MonitorRuleSetInput):
    sample_titles: list[PreviewSampleInput] = Field(min_length=1, max_length=20)


class MonitorRuleSetView(OutputModel):
    model_config = ConfigDict(frozen=True)

    match_any: list[str]
    match_all: list[str]
    exclude: list[str]


class MonitorRulePreviewSampleView(OutputModel):
    sample_index: int = Field(ge=0)
    matched: bool
    excluded_by: list[str]


class MonitorExpansionPreviewView(OutputModel):
    local_alias_external_queries: Literal[0]
    local_alias_budget_units: Literal[0]
    upstream_status: Literal["pending_source_selection"]
    upstream_external_queries: None
    upstream_budget_units: None


class MonitorTopicPreviewView(OutputModel):
    rules: MonitorRuleSetView
    samples: list[MonitorRulePreviewSampleView]
    expansion: MonitorExpansionPreviewView


class MonitorTopicView(OutputModel):
    id: UUID
    name: str
    status: MonitorTopicStatus
    readiness_status: MonitorTopicReadinessStatus
    current_version: int = Field(ge=1)
    rules: MonitorRuleSetView
    created_at: datetime
    updated_at: datetime
