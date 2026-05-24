from __future__ import annotations

from fastapi import APIRouter, Depends
from sqlalchemy.orm import Session

from server.app.db.session import get_session
from server.app.schemas.search import SearchCreate, SearchRead
from server.app.services.search import search_sources

router = APIRouter(prefix="/api/search", tags=["search"])


@router.post("", response_model=SearchRead)
def search(payload: SearchCreate, session: Session = Depends(get_session)) -> SearchRead:
    return search_sources(session, query=payload.query, source_types=payload.source_types, limit=payload.limit)
