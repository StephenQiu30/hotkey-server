from __future__ import annotations

from fastapi import APIRouter, Depends, Query
from sqlalchemy.orm import Session

from server.app.db.session import get_session
from server.app.schemas.analytics import (
    AnalyticsSentimentPoint,
    AnalyticsSentimentResponse,
    AnalyticsSourceResponse,
    AnalyticsTrendResponse,
)
from server.app.services import analytics as analytics_service

router = APIRouter(prefix="/api/analytics", tags=["analytics"])


@router.get("/trend", response_model=AnalyticsTrendResponse)
def trend(session: Session = Depends(get_session), days: int = Query(default=14, ge=7, le=90)) -> AnalyticsTrendResponse:
    points = analytics_service.get_trend(session, days=days)
    return AnalyticsTrendResponse(period_days=days, points=[point for point in points])


@router.get("/sources", response_model=AnalyticsSourceResponse)
def sources(
    session: Session = Depends(get_session),
    days: int = Query(default=14, ge=7, le=90),
    limit: int = Query(default=8, ge=1, le=20),
) -> AnalyticsSourceResponse:
    items = analytics_service.get_top_sources(session, days=days, limit=limit)
    return AnalyticsSourceResponse(period_days=days, limit=limit, items=[_normalize_source(item) for item in items])


@router.get("/sentiment", response_model=AnalyticsSentimentResponse)
def sentiment(session: Session = Depends(get_session), days: int = Query(default=14, ge=7, le=90)) -> AnalyticsSentimentResponse:
    by_importance = analytics_service.get_sentiment(session, days=days)
    items = [AnalyticsSentimentPoint(importance=key, count=value) for key, value in by_importance.items()]
    return AnalyticsSentimentResponse(period_days=days, total=sum(by_importance.values()), by_importance=items)


def _normalize_source(item: dict[str, str | int]) -> dict[str, int | str]:
    return {
        "source_id": int(item["source_id"]),
        "source_name": str(item["source_name"]),
        "hotspot_count": int(item["hotspot_count"]),
        "active_count": int(item["active_count"]),
        "filtered_count": int(item["filtered_count"]),
    }
