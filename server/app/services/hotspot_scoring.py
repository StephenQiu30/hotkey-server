from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from decimal import Decimal, InvalidOperation
from typing import Any

from server.app.core.settings import settings
from server.app.models.hotspot import Hotspot


def _clamp_score(value: float, *, min_value: float = 0.0, max_value: float = 100.0) -> float:
    try:
        value_num = float(value)
    except (TypeError, ValueError):
        return min_value
    if value_num < min_value:
        return min_value
    if value_num > max_value:
        return max_value
    return round(value_num, 2)


def _to_float(value: object, *, default: float = 0.0) -> float:
    if isinstance(value, bool):
        return float(int(value))
    if isinstance(value, int | float):
        return float(value)
    if isinstance(value, Decimal):
        try:
            return float(value)
        except (InvalidOperation, ValueError, TypeError):
            return default
    if isinstance(value, str):
        try:
            return float(value)
        except ValueError:
            return default
    return default


@dataclass(slots=True)
class HotnessBreakdown:
    ai_relevance: float
    freshness: float
    source_strength: float
    keyword_fit: float

    def as_dict(self) -> dict[str, float]:
        return {
            "ai_relevance": self.ai_relevance,
            "freshness": self.freshness,
            "source_strength": self.source_strength,
            "keyword_fit": self.keyword_fit,
        }


@dataclass(slots=True)
class HotnessDecision:
    score: float
    version: int
    breakdown: HotnessBreakdown
    reason: str

    def raw_payload(self) -> dict[str, Any]:
        return {
            "hotness_version": self.version,
            "hotness_score": self.score,
            "hotness_reason": self.reason,
            "hotness_breakdown": self.breakdown.as_dict(),
        }


def _freshed_time_score(event_time: datetime | None, now: datetime | None = None) -> float:
    if event_time is None:
        return 20.0
    now = now or datetime.now(timezone.utc)
    delta_hours = max(0.0, (now - event_time).total_seconds() / 3600.0)
    if delta_hours <= 1.0:
        return 100.0
    if delta_hours >= settings.hotness_min_freshness_hours:
        return 0.0
    score = 100.0 * (1 - delta_hours / settings.hotness_min_freshness_hours)
    return _clamp_score(score)


def _source_strength_score(source_strength: float) -> float:
    normalized = _clamp_score(source_strength, min_value=0.0, max_value=100.0)
    return normalized


def compute_hotness_score(
    *,
    hotspot: Hotspot,
    analysis: object,
    source_strength: float | None = None,
    trust_penalty: float = 0.0,
    version: int = 1,
) -> HotnessDecision:
    """Compute v1 hotness using relevance/freshness/source/keyword evidence."""
    ai_relevance = _clamp_score(_to_float(getattr(analysis, "relevance_score", 0.0), default=0.0))
    now = datetime.now(timezone.utc)
    freshness = _freshed_time_score(hotspot.published_at, now=now)
    if source_strength is None:
        source_strength = _to_float(hotspot.source.config.get("source_strength", settings.hotness_source_strength_default), default=0.0)
    keyword_fit = 100.0 if bool(getattr(analysis, "keyword_mentioned", False)) else 60.0
    source_strength_score = _source_strength_score(source_strength)

    score = (
        ai_relevance * 0.55
        + freshness * 0.20
        + source_strength_score * 0.15
        + keyword_fit * 0.10
    )
    score = _clamp_score(score - trust_penalty, min_value=0.0, max_value=settings.hotness_max_score)

    reason = (
        f"AI相关性={ai_relevance:.2f}, 时效={freshness:.2f}, 来源强度={source_strength_score:.2f}, "
        f"关键词匹配={keyword_fit:.2f}, 惩罚={trust_penalty:.2f}"
    )
    return HotnessDecision(
        score=score,
        version=version,
        breakdown=HotnessBreakdown(
            ai_relevance=ai_relevance,
            freshness=freshness,
            source_strength=source_strength_score,
            keyword_fit=keyword_fit,
        ),
        reason=reason,
    )
