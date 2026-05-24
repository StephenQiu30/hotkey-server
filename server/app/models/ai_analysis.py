from __future__ import annotations

from datetime import datetime
from decimal import Decimal
from typing import TYPE_CHECKING, Any

from sqlalchemy import Boolean, CheckConstraint, DateTime, ForeignKey, Numeric, Text, func
from sqlalchemy import JSON as PortableJSON
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column, relationship

from server.app.db.base import Base

if TYPE_CHECKING:
    from server.app.models.hotspot import Hotspot


class AiAnalysis(Base):
    __tablename__ = "ai_analyses"
    __table_args__ = (CheckConstraint("relevance_score >= 0 AND relevance_score <= 100", name="ck_ai_analyses_relevance_score"),)

    id: Mapped[int] = mapped_column(primary_key=True)
    hotspot_id: Mapped[int] = mapped_column(ForeignKey("hotspots.id", ondelete="CASCADE"), unique=True, nullable=False)
    is_real: Mapped[bool | None] = mapped_column(Boolean)
    relevance_score: Mapped[Decimal] = mapped_column(Numeric(5, 2), nullable=False, server_default="0")
    relevance_reason: Mapped[str | None] = mapped_column(Text)
    keyword_mentioned: Mapped[bool] = mapped_column(Boolean, nullable=False, server_default="false")
    importance: Mapped[str] = mapped_column(Text, nullable=False, server_default="medium")
    summary: Mapped[str | None] = mapped_column(Text)
    model_name: Mapped[str | None] = mapped_column(Text)
    raw_response: Mapped[dict[str, Any]] = mapped_column(
        PortableJSON().with_variant(JSONB, "postgresql"),
        nullable=False,
        server_default="{}",
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())

    hotspot: Mapped[Hotspot] = relationship(back_populates="ai_analysis")

    @property
    def quick_understanding(self) -> list[str]:
        value = self.raw_response.get("quick_understanding") if isinstance(self.raw_response, dict) else None
        if not isinstance(value, list):
            return []
        return [str(item) for item in value if str(item).strip()]

    @property
    def topic_ideas(self) -> list[dict[str, str]]:
        value = self.raw_response.get("topic_ideas") if isinstance(self.raw_response, dict) else None
        if not isinstance(value, list):
            return []
        ideas: list[dict[str, str]] = []
        for item in value:
            if not isinstance(item, dict):
                continue
            title = str(item.get("title") or "").strip()
            if not title:
                continue
            ideas.append(
                {
                    "title": title,
                    "angle": str(item.get("angle") or "").strip(),
                    "format": str(item.get("format") or "").strip(),
                    "rationale": str(item.get("rationale") or "").strip(),
                }
            )
        return ideas

    @property
    def truth_score(self) -> float | None:
        if not isinstance(self.raw_response, dict):
            return None
        value = self.raw_response.get("truth_score")
        if value is None:
            return None
        try:
            return float(value)
        except (TypeError, ValueError):
            return None

    @property
    def source_risk_level(self) -> str | None:
        if not isinstance(self.raw_response, dict):
            return None
        value = self.raw_response.get("source_risk_level")
        return str(value) if isinstance(value, str) else None

    @property
    def source_risk_tags(self) -> list[str]:
        if not isinstance(self.raw_response, dict):
            return []
        value = self.raw_response.get("source_risk_tags")
        if not isinstance(value, list):
            return []
        return [str(item) for item in value]

    @property
    def source_evidence_bundle(self) -> dict[str, Any]:
        if not isinstance(self.raw_response, dict):
            return {}
        value = self.raw_response.get("source_evidence_bundle")
        return value if isinstance(value, dict) else {}

    @property
    def source_evidence_version(self) -> int:
        if not isinstance(self.raw_response, dict):
            return 0
        value = self.raw_response.get("source_evidence_version")
        if value is None:
            return 0
        try:
            return int(value)
        except (TypeError, ValueError):
            return 0

    @property
    def hotness_version(self) -> int | None:
        if not isinstance(self.raw_response, dict):
            return None
        value = self.raw_response.get("hotness_version")
        if value is None:
            return None
        try:
            return int(value)
        except (TypeError, ValueError):
            return None

    @property
    def hotness_breakdown(self) -> dict[str, Any]:
        if not isinstance(self.raw_response, dict):
            return {}
        value = self.raw_response.get("hotness_breakdown")
        return value if isinstance(value, dict) else {}

    @property
    def hotness_reason(self) -> str | None:
        if not isinstance(self.raw_response, dict):
            return None
        value = self.raw_response.get("hotness_reason")
        return str(value) if isinstance(value, str) else None

    @property
    def hotness_score(self) -> float | None:
        if not isinstance(self.raw_response, dict):
            return None
        value = self.raw_response.get("hotness_score")
        if value is None:
            return None
        try:
            return float(value)
        except (TypeError, ValueError):
            return None
