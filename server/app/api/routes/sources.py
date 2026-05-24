from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from server.app.db.session import get_session
from server.app.core.security import require_permission
from server.app.models.source import Source
from server.app.schemas.source import SourceCreate, SourceRead, SourceUpdate

router = APIRouter(prefix="/api/sources", tags=["sources"])


@router.get("", response_model=list[SourceRead])
def list_sources(session: Session = Depends(get_session)) -> list[Source]:
    return list(session.scalars(select(Source).order_by(Source.id)))


@router.post("", response_model=SourceRead, status_code=201, dependencies=[Depends(require_permission("source.manage"))])
def create_source(payload: SourceCreate, session: Session = Depends(get_session)) -> Source:
    source = Source(**payload.model_dump())
    session.add(source)
    try:
        session.commit()
    except IntegrityError as exc:
        session.rollback()
        raise HTTPException(status_code=409, detail="Source already exists.") from exc
    session.refresh(source)
    return source


@router.patch("/{source_id}", response_model=SourceRead, dependencies=[Depends(require_permission("source.manage"))])
def update_source(source_id: int, payload: SourceUpdate, session: Session = Depends(get_session)) -> Source:
    source = _get_source(session, source_id)
    for key, value in payload.model_dump(exclude_unset=True).items():
        setattr(source, key, value)
    try:
        session.commit()
    except IntegrityError as exc:
        session.rollback()
        raise HTTPException(status_code=409, detail="Source already exists.") from exc
    session.refresh(source)
    return source


@router.delete("/{source_id}", status_code=204, dependencies=[Depends(require_permission("source.manage"))])
def delete_source(source_id: int, session: Session = Depends(get_session)) -> None:
    source = _get_source(session, source_id)
    session.delete(source)
    session.commit()


@router.post("/{source_id}/toggle", response_model=SourceRead, dependencies=[Depends(require_permission("source.manage"))])
def toggle_source(source_id: int, session: Session = Depends(get_session)) -> Source:
    source = _get_source(session, source_id)
    source.enabled = not source.enabled
    session.commit()
    session.refresh(source)
    return source


def _get_source(session: Session, source_id: int) -> Source:
    source = session.get(Source, source_id)
    if source is None:
        raise HTTPException(status_code=404, detail="Source not found.")
    return source
