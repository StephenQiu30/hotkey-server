from __future__ import annotations

from datetime import datetime, timedelta, timezone
from typing import TypedDict

from sqlalchemy import case, func, select
from sqlalchemy.orm import Session

from server.app.models.ai_analysis import AiAnalysis
from server.app.models.hotspot import Hotspot
from server.app.models.source import Source


class TrendPoint(TypedDict):
    date: str
    total_count: int
    active_count: int
    filtered_count: int


def get_trend(session: Session, days: int = 14) -> list[TrendPoint]:
    now = datetime.now(timezone.utc).replace(hour=0, minute=0, second=0, microsecond=0)
    start = now - timedelta(days=max(days - 1, 0))
    rows = session.execute(_trend_query(start)).all()
    day_index: dict[str, dict[str, int]] = {}
    for row in rows:
        day = row.day.isoformat()
        day_index[day] = {
            "total_count": int(row.total_count),
            "active_count": int(row.active_count or 0),
            "filtered_count": int(row.filtered_count or 0),
        }
    result: list[TrendPoint] = []
    for offset in range((now.date() - start.date()).days + 1):
        day = start + timedelta(days=offset)
        key = day.date().isoformat()
        values = day_index.get(key, {"total_count": 0, "active_count": 0, "filtered_count": 0})
        result.append(
            {
                "date": key,
                "total_count": values["total_count"],
                "active_count": values["active_count"],
                "filtered_count": values["filtered_count"],
            }
        )
    return result


def get_top_sources(session: Session, days: int = 14, limit: int = 10) -> list[dict[str, int | str]]:
    start = datetime.now(timezone.utc) - timedelta(days=max(days - 1, 0))
    rows = session.execute(_top_sources_query(start).limit(limit)).all()
    return [
        {
            "source_id": int(row.source_id),
            "source_name": str(row.source_name),
            "hotspot_count": int(row.hotspot_count),
            "active_count": int(row.active_count or 0),
            "filtered_count": int(row.filtered_count or 0),
        }
        for row in rows
    ]


def get_sentiment(session: Session, days: int = 14) -> dict[str, int]:
    start = datetime.now(timezone.utc) - timedelta(days=max(days - 1, 0))
    rows = session.execute(_sentiment_query(start)).all()
    distribution = {"high": 0, "medium": 0, "low": 0}
    for row in rows:
        if row.importance in distribution:
            distribution[row.importance] = int(row.count)
        else:
            distribution[str(row.importance)] = distribution.get(str(row.importance), 0) + int(row.count)
    return distribution


def _trend_query(start: datetime):
    return (
        select(
            func.date(Hotspot.fetched_at).label("day"),
            func.count(Hotspot.id).label("total_count"),
            func.sum(case((Hotspot.status == "active", 1), else_=0)).label("active_count"),
            func.sum(case((Hotspot.status == "filtered", 1), else_=0)).label("filtered_count"),
        )
        .where(Hotspot.fetched_at >= start)
        .group_by(func.date(Hotspot.fetched_at))
        .order_by(func.date(Hotspot.fetched_at))
    )


def _top_sources_query(start: datetime):
    return (
        select(
            Source.id.label("source_id"),
            Source.name.label("source_name"),
            func.count(Hotspot.id).label("hotspot_count"),
            func.sum(case((Hotspot.status == "active", 1), else_=0)).label("active_count"),
            func.sum(case((Hotspot.status == "filtered", 1), else_=0)).label("filtered_count"),
        )
        .join(Hotspot, Hotspot.source_id == Source.id)
        .where(Hotspot.fetched_at >= start)
        .group_by(Source.id, Source.name)
        .order_by(func.count(Hotspot.id).desc())
    )


def _sentiment_query(start: datetime):
    return (
        select(AiAnalysis.importance.label("importance"), func.count(AiAnalysis.id).label("count"))
        .join(Hotspot, Hotspot.id == AiAnalysis.hotspot_id)
        .where(Hotspot.status == "active", Hotspot.fetched_at >= start)
        .group_by(AiAnalysis.importance)
    )
