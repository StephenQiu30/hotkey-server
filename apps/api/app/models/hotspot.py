from __future__ import annotations

from datetime import datetime
from typing import TYPE_CHECKING, Any

from sqlalchemy import DateTime, ForeignKey, Text, UniqueConstraint, func
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column, relationship

from apps.api.app.db.base import Base

if TYPE_CHECKING:
    from apps.api.app.models.ai_analysis import AiAnalysis
    from apps.api.app.models.keyword import Keyword
    from apps.api.app.models.notification import Notification
    from apps.api.app.models.source import Source


class Hotspot(Base):
    __tablename__ = "hotspots"
    __table_args__ = (UniqueConstraint("source_id", "url", name="uq_hotspots_source_url"),)

    id: Mapped[int] = mapped_column(primary_key=True)
    title: Mapped[str] = mapped_column(Text, nullable=False)
    url: Mapped[str] = mapped_column(Text, nullable=False)
    source_id: Mapped[int] = mapped_column(ForeignKey("sources.id", ondelete="RESTRICT"), nullable=False)
    keyword_id: Mapped[int | None] = mapped_column(ForeignKey("keywords.id", ondelete="SET NULL"))
    author: Mapped[str | None] = mapped_column(Text)
    snippet: Mapped[str | None] = mapped_column(Text)
    published_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    fetched_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    status: Mapped[str] = mapped_column(Text, nullable=False, server_default="new")
    raw_payload: Mapped[dict[str, Any]] = mapped_column(JSONB, nullable=False, server_default="{}")
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())

    source: Mapped[Source] = relationship(back_populates="hotspots")
    keyword: Mapped[Keyword | None] = relationship(back_populates="hotspots")
    ai_analysis: Mapped[AiAnalysis | None] = relationship(back_populates="hotspot", uselist=False)
    notifications: Mapped[list[Notification]] = relationship(back_populates="hotspot")

    @property
    def cluster_id(self) -> str | None:
        value = self.raw_payload.get("cluster_id") if isinstance(self.raw_payload, dict) else None
        return str(value) if value else None

    @property
    def trend_score(self) -> float:
        if not isinstance(self.raw_payload, dict):
            return 0.0
        for key in ("trend_score", "heat_score", "points", "score", "stars", "stargazers_count"):
            value = self.raw_payload.get(key)
            if isinstance(value, int | float):
                return max(0.0, min(float(value), 100.0))
        return 0.0

    @property
    def rank_score(self) -> float:
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("rank_score"), int | float):
            return max(0.0, min(float(self.raw_payload["rank_score"]), 100.0))
        relevance_score = 0.0
        if self.ai_analysis is not None:
            relevance_score = float(self.ai_analysis.relevance_score)
        return round(max(0.0, min(relevance_score * 0.75 + self.trend_score * 0.25, 100.0)), 2)
