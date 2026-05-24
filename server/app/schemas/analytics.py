from __future__ import annotations

from datetime import date
from pydantic import BaseModel


class AnalyticsTrendPoint(BaseModel):
    date: date
    total_count: int
    active_count: int
    filtered_count: int


class AnalyticsTrendResponse(BaseModel):
    period_days: int
    points: list[AnalyticsTrendPoint]


class AnalyticsSourceStat(BaseModel):
    source_id: int
    source_name: str
    hotspot_count: int
    active_count: int
    filtered_count: int


class AnalyticsSourceResponse(BaseModel):
    period_days: int
    limit: int
    items: list[AnalyticsSourceStat]


class AnalyticsSentimentPoint(BaseModel):
    importance: str
    count: int


class AnalyticsSentimentResponse(BaseModel):
    period_days: int
    total: int
    by_importance: list[AnalyticsSentimentPoint]
