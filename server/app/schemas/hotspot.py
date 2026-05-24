from __future__ import annotations

from datetime import datetime
from typing import Any

from pydantic import BaseModel, ConfigDict, Field

from server.app.schemas.ai_analysis import AiAnalysisRead
from server.app.schemas.keyword import KeywordRead
from server.app.schemas.source import SourceRead


class HotspotRead(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    title: str
    url: str
    source_id: int
    keyword_id: int | None
    author: str | None
    snippet: str | None
    published_at: datetime | None
    fetched_at: datetime
    status: str
    cluster_id: str | None = None
    cluster_version: int | None = None
    rank_score: float = 0
    trend_score: float = 0
    hotness_score: float = 0
    hotness_version: int = 1
    hotness_reason: str | None = None
    hotness_breakdown: dict[str, Any] = Field(default_factory=dict)
    source_risk_level: str | None = None
    source_risk_tags: list[str] = Field(default_factory=list)
    source_risk_badge: str | None = None
    source_evidence_bundle: dict[str, Any] = Field(default_factory=dict)
    source_evidence_version: int = 0
    source_selected: str | None = None
    source_selected_type: str | None = None
    source_fallback: dict[str, Any] = Field(default_factory=dict)
    raw_payload: dict[str, Any]
    created_at: datetime
    updated_at: datetime
    source: SourceRead | None = None
    keyword: KeywordRead | None = None
    ai_analysis: AiAnalysisRead | None = None


class HotspotClusterResponse(BaseModel):
    cluster_id: str
    cluster_size: int
    items: list[HotspotRead]
