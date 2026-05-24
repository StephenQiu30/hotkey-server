from __future__ import annotations

from datetime import datetime
from typing import Any

from pydantic import BaseModel, ConfigDict

from apps.api.app.schemas.ai_analysis import AiAnalysisRead
from apps.api.app.schemas.keyword import KeywordRead
from apps.api.app.schemas.source import SourceRead


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
    rank_score: float = 0
    trend_score: float = 0
    raw_payload: dict[str, Any]
    created_at: datetime
    updated_at: datetime
    source: SourceRead | None = None
    keyword: KeywordRead | None = None
    ai_analysis: AiAnalysisRead | None = None
