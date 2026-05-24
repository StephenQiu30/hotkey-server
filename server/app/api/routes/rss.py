from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException, Query
from fastapi.responses import Response
from sqlalchemy.orm import Session

from server.app.core.settings import settings
from server.app.db.session import get_session
from server.app.services import rss as rss_service

router = APIRouter(tags=["rss"])


@router.get("/rss/trending")
def trending_rss(
    limit: int = Query(default=50, ge=1, le=200),
    session: Session = Depends(get_session),
) -> Response:
    content = rss_service.generate_trending_feed(session, limit=limit)
    return Response(content=content, media_type="application/xml")


@router.get("/rss/keyword/{keyword_name}")
def keyword_rss(
    keyword_name: str,
    limit: int = Query(default=50, ge=1, le=200),
    token: str | None = None,
    session: Session = Depends(get_session),
) -> Response:
    if settings.rss_access_token and token != settings.rss_access_token:
        raise HTTPException(status_code=403, detail="RSS token invalid.")
    content = rss_service.generate_keyword_feed(session, keyword_name=keyword_name, limit=limit)
    return Response(content=content, media_type="application/xml")


@router.get("/rss/user/{user_id}")
def user_rss(
    user_id: int,
    limit: int = Query(default=50, ge=1, le=200),
    token: str | None = None,
    session: Session = Depends(get_session),
) -> Response:
    if settings.rss_access_token and token != settings.rss_access_token:
        raise HTTPException(status_code=403, detail="RSS token invalid.")
    content = rss_service.generate_user_feed(session=session, user_id=user_id, limit=limit)
    return Response(content=content, media_type="application/xml")
