from __future__ import annotations

from datetime import datetime
from decimal import Decimal
from typing import Any

from pydantic import BaseModel, ConfigDict, Field


class TopicIdeaRead(BaseModel):
    title: str
    angle: str = ""
    format: str = ""
    rationale: str = ""


class AiAnalysisRead(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    hotspot_id: int
    is_real: bool | None
    relevance_score: Decimal
    relevance_reason: str | None
    keyword_mentioned: bool
    importance: str
    summary: str | None
    truth_score: float | None = None
    source_risk_level: str | None = None
    source_risk_tags: list[str] = Field(default_factory=list)
    source_evidence_bundle: dict[str, Any] = Field(default_factory=dict)
    source_evidence_version: int | None = None
    hotness_score: float | None = None
    hotness_version: int | None = None
    hotness_breakdown: dict[str, Any] = Field(default_factory=dict)
    hotness_reason: str | None = None
    provider_trace: list[dict[str, Any]] = Field(default_factory=list)
    quick_understanding: list[str] = Field(default_factory=list)
    topic_ideas: list[TopicIdeaRead] = Field(default_factory=list)
    model_name: str | None
    raw_response: dict[str, Any]
    created_at: datetime
    updated_at: datetime
