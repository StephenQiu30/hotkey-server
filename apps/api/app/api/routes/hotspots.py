from __future__ import annotations

from datetime import datetime

from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import Integer, Numeric, Select, cast, func, select
from sqlalchemy.orm import Session, selectinload

from apps.api.app.db.session import get_session
from apps.api.app.models.ai_analysis import AiAnalysis
from apps.api.app.models.hotspot import Hotspot
from apps.api.app.schemas.hotspot import HotspotClusterResponse, HotspotRead

router = APIRouter(prefix="/api/hotspots", tags=["hotspots"])


@router.get("", response_model=dict)
def list_hotspots(
    keyword_id: int | None = None,
    source_id: int | None = None,
    importance: str | None = None,
    published_from: datetime | None = None,
    published_to: datetime | None = None,
    sort: str = "fetched_at_desc",
    limit: int = Query(default=50, ge=1, le=200),
    offset: int = Query(default=0, ge=0),
    session: Session = Depends(get_session),
) -> dict:
    stmt = _base_hotspot_query()
    if keyword_id is not None:
        stmt = stmt.where(Hotspot.keyword_id == keyword_id)
    if source_id is not None:
        stmt = stmt.where(Hotspot.source_id == source_id)
    if published_from is not None:
        stmt = stmt.where(Hotspot.published_at >= published_from)
    if published_to is not None:
        stmt = stmt.where(Hotspot.published_at <= published_to)
    if importance:
        stmt = stmt.where(Hotspot.ai_analysis.has(AiAnalysis.importance == importance))
    stmt = _apply_sort(stmt, sort).limit(limit).offset(offset)
    items = list(session.scalars(stmt).unique())
    return {"items": [HotspotRead.model_validate(item).model_dump(mode="json") for item in items], "limit": limit, "offset": offset}


@router.get("/{hotspot_id}", response_model=HotspotRead)
def get_hotspot(hotspot_id: int, session: Session = Depends(get_session)) -> Hotspot:
    hotspot = session.scalar(_base_hotspot_query().where(Hotspot.id == hotspot_id))
    if hotspot is None:
        raise HTTPException(status_code=404, detail="Hotspot not found.")
    return hotspot


@router.get("/cluster/{cluster_id}", response_model=HotspotClusterResponse)
def get_hotspot_cluster(
    cluster_id: str,
    limit: int = Query(default=50, ge=1, le=200),
    offset: int = Query(default=0, ge=0),
    session: Session = Depends(get_session),
) -> HotspotClusterResponse:
    if not isinstance(limit, int):
        limit = 50
    if not isinstance(offset, int):
        offset = 0
    stmt = _cluster_scope_query(cluster_id).limit(limit).offset(offset)
    items = list(session.scalars(stmt).unique())
    cluster_size = session.scalar(_cluster_scope_count(cluster_id)) or 0

    return HotspotClusterResponse(
        cluster_id=cluster_id,
        cluster_size=cluster_size,
        items=[HotspotRead.model_validate(item) for item in items],
    )


@router.get("/{hotspot_id}/cluster-history", response_model=HotspotClusterResponse)
def get_hotspot_cluster_history(
    hotspot_id: int,
    session: Session = Depends(get_session),
) -> HotspotClusterResponse:
    hotspot = session.scalar(_base_hotspot_query().where(Hotspot.id == hotspot_id))
    if hotspot is None:
        raise HTTPException(status_code=404, detail="Hotspot not found.")

    cluster_id = hotspot.cluster_id
    if not cluster_id:
        raise HTTPException(status_code=404, detail="Cluster metadata missing.")

    return get_hotspot_cluster(cluster_id, session=session)


def _base_hotspot_query() -> Select:
    return select(Hotspot).options(
        selectinload(Hotspot.source),
        selectinload(Hotspot.keyword),
        selectinload(Hotspot.ai_analysis),
    )


def _cluster_scope_query(cluster_id: str) -> Select:
    return (
        _base_hotspot_query()
        .where(Hotspot.raw_payload["cluster_id"].as_string() == cluster_id)
        .order_by(_cluster_version_expr().asc().nullslast(), Hotspot.created_at.asc(), Hotspot.id.asc())
    )


def _cluster_scope_count(cluster_id: str):
    return (
        select(func.count())
        .select_from(Hotspot)
        .where(Hotspot.raw_payload["cluster_id"].as_string() == cluster_id)
    )


def _cluster_version_expr():
    return cast(Hotspot.raw_payload["cluster_version"], Integer)


def _apply_sort(stmt: Select, sort: str) -> Select:
    trend_score = cast(Hotspot.raw_payload["trend_score"], Numeric(8, 2))
    if sort == "rank_score_desc":
        return stmt.outerjoin(AiAnalysis, AiAnalysis.hotspot_id == Hotspot.id).order_by(AiAnalysis.relevance_score.desc().nullslast(), trend_score.desc().nullslast(), Hotspot.id.desc())
    if sort == "trend_score_desc":
        return stmt.order_by(trend_score.desc().nullslast(), Hotspot.fetched_at.desc(), Hotspot.id.desc())
    if sort == "published_at_asc":
        return stmt.order_by(Hotspot.published_at.asc().nullslast(), Hotspot.id.desc())
    if sort == "relevance_desc":
        return stmt.outerjoin(AiAnalysis, AiAnalysis.hotspot_id == Hotspot.id).order_by(AiAnalysis.relevance_score.desc().nullslast(), Hotspot.id.desc())
    if sort == "importance_desc":
        return stmt.outerjoin(AiAnalysis, AiAnalysis.hotspot_id == Hotspot.id).order_by(AiAnalysis.importance.desc().nullslast(), Hotspot.id.desc())
    if sort == "published_at_desc":
        return stmt.order_by(Hotspot.published_at.desc().nullslast(), Hotspot.id.desc())
    return stmt.order_by(Hotspot.fetched_at.desc(), Hotspot.id.desc())
