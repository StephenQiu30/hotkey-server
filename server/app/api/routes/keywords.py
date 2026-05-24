from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from server.app.db.session import get_session
from server.app.core.security import require_permission
from server.app.models.keyword import Keyword
from server.app.schemas.keyword import KeywordCreate, KeywordRead, KeywordUpdate

router = APIRouter(prefix="/api/keywords", tags=["keywords"])


@router.get("", response_model=list[KeywordRead])
def list_keywords(session: Session = Depends(get_session)) -> list[Keyword]:
    return list(session.scalars(select(Keyword).order_by(Keyword.priority.desc(), Keyword.id)))


@router.post("", response_model=KeywordRead, status_code=201, dependencies=[Depends(require_permission("keyword.manage"))])
def create_keyword(payload: KeywordCreate, session: Session = Depends(get_session)) -> Keyword:
    keyword = Keyword(**payload.model_dump())
    session.add(keyword)
    try:
        session.commit()
    except IntegrityError as exc:
        session.rollback()
        raise HTTPException(status_code=409, detail="Keyword already exists.") from exc
    session.refresh(keyword)
    return keyword


@router.patch("/{keyword_id}", response_model=KeywordRead, dependencies=[Depends(require_permission("keyword.manage"))])
def update_keyword(keyword_id: int, payload: KeywordUpdate, session: Session = Depends(get_session)) -> Keyword:
    keyword = _get_keyword(session, keyword_id)
    for key, value in payload.model_dump(exclude_unset=True).items():
        setattr(keyword, key, value)
    try:
        session.commit()
    except IntegrityError as exc:
        session.rollback()
        raise HTTPException(status_code=409, detail="Keyword already exists.") from exc
    session.refresh(keyword)
    return keyword


@router.delete("/{keyword_id}", status_code=204, dependencies=[Depends(require_permission("keyword.manage"))])
def delete_keyword(keyword_id: int, session: Session = Depends(get_session)) -> None:
    keyword = _get_keyword(session, keyword_id)
    session.delete(keyword)
    session.commit()


@router.post("/{keyword_id}/toggle", response_model=KeywordRead, dependencies=[Depends(require_permission("keyword.manage"))])
def toggle_keyword(keyword_id: int, session: Session = Depends(get_session)) -> Keyword:
    keyword = _get_keyword(session, keyword_id)
    keyword.enabled = not keyword.enabled
    session.commit()
    session.refresh(keyword)
    return keyword


def _get_keyword(session: Session, keyword_id: int) -> Keyword:
    keyword = session.get(Keyword, keyword_id)
    if keyword is None:
        raise HTTPException(status_code=404, detail="Keyword not found.")
    return keyword
