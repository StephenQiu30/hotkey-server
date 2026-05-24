from __future__ import annotations

from datetime import datetime
from typing import TYPE_CHECKING, Any

from sqlalchemy import DateTime, ForeignKey, Text, UniqueConstraint, func
from sqlalchemy import JSON as PortableJSON
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column, relationship

from server.app.db.base import Base
from server.app.core.settings import settings

if TYPE_CHECKING:
    from server.app.models.ai_analysis import AiAnalysis
    from server.app.models.keyword import Keyword
    from server.app.models.notification import Notification
    from server.app.models.source import Source


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
    raw_payload: Mapped[dict[str, Any]] = mapped_column(
        PortableJSON().with_variant(JSONB, "postgresql"),
        nullable=False,
        server_default="{}",
    )
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
    def cluster_version(self) -> int | None:
        if not isinstance(self.raw_payload, dict):
            return None
        version = self.raw_payload.get("cluster_version")
        if version is None:
            return None
        try:
            return int(version)
        except (TypeError, ValueError):
            return None

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

    @property
    def hotness_score(self) -> float:
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("hotness_score"), int | float):
            return max(0.0, min(float(self.raw_payload["hotness_score"]), settings.hotness_max_score))
        if self.ai_analysis is None:
            return self.rank_score
        hotness = self.ai_analysis.hotness_score
        if hotness is None:
            return self.rank_score
        return max(0.0, min(float(hotness), settings.hotness_max_score))

    @property
    def hotness_version(self) -> int:
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("hotness_version"), int | float):
            return int(self.raw_payload.get("hotness_version"))
        if self.ai_analysis is None:
            return 1
        return int(self.ai_analysis.hotness_version or 1)

    @property
    def hotness_breakdown(self) -> dict[str, Any]:
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("hotness_breakdown"), dict):
            return self.raw_payload["hotness_breakdown"]  # type: ignore[return-value]
        if self.ai_analysis is None:
            return {}
        return dict(self.ai_analysis.hotness_breakdown or {})

    @property
    def hotness_reason(self) -> str | None:
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("hotness_reason"), str):
            return self.raw_payload["hotness_reason"]
        if self.ai_analysis is None:
            return None
        return self.ai_analysis.hotness_reason

    @property
    def source_risk_level(self) -> str:
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("source_risk_level"), str):
            return self.raw_payload["source_risk_level"]
        if self.ai_analysis is None:
            return "medium"
        return self.ai_analysis.source_risk_level or "medium"

    @property
    def source_risk_tags(self) -> list[str]:
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("source_risk_tags"), list):
            return list(self.raw_payload["source_risk_tags"])
        if self.ai_analysis is None:
            return []
        return list(self.ai_analysis.source_risk_tags)

    @property
    def source_risk_badge(self) -> str | None:
        # Low-trust events stay listable for review, but the response carries a stable display badge
        # so clients and reports can keep them visibly risk-marked instead of silently promoting them.
        if self.source_risk_level == "low":
            return "low_trust_source"
        if self.source_risk_tags:
            return "source_risk_review"
        return None

    @property
    def source_evidence_bundle(self) -> dict[str, Any]:
        # Evidence is persisted both on hotspot.raw_payload and AiAnalysis; raw payload fallback keeps
        # audit fields readable even when analysis is absent or not eagerly loaded.
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("source_evidence_bundle"), dict):
            return self.raw_payload["source_evidence_bundle"]  # type: ignore[return-value]
        if self.ai_analysis is None:
            return {}
        return dict(self.ai_analysis.source_evidence_bundle)

    @property
    def source_evidence_version(self) -> int:
        if isinstance(self.raw_payload, dict):
            version = self.raw_payload.get("source_evidence_version")
            if isinstance(version, int | float):
                return int(version)
        if self.ai_analysis is None:
            return 0
        return int(self.ai_analysis.source_evidence_version or 0)

    @property
    def source_selected(self) -> str | None:
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("source_selected"), str):
            return self.raw_payload["source_selected"]
        return None

    @property
    def source_selected_type(self) -> str | None:
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("source_selected_type"), str):
            return self.raw_payload["source_selected_type"]
        return None

    @property
    def source_fallback(self) -> dict[str, Any]:
        # Expose source routing audit data as schema-safe defaults so fallback evidence stays API-readable.
        if isinstance(self.raw_payload, dict) and isinstance(self.raw_payload.get("source_fallback"), dict):
            return self.raw_payload["source_fallback"]  # type: ignore[return-value]
        return {}
