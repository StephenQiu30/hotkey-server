from __future__ import annotations

from fastapi import APIRouter, Depends, Query
from sqlalchemy import select
from sqlalchemy.orm import Session

from apps.api.app.db.session import get_session
from apps.api.app.models.notification import Notification
from apps.api.app.schemas.notification import NotificationListResponse, NotificationRead

router = APIRouter(prefix="/api/notifications", tags=["notifications"])


@router.get("", response_model=NotificationListResponse)
def list_notifications(
    status: str | None = None,
    limit: int = Query(default=50, ge=1, le=200),
    offset: int = Query(default=0, ge=0),
    session: Session = Depends(get_session),
) -> NotificationListResponse:
    stmt = select(Notification).order_by(Notification.created_at.desc()).limit(limit).offset(offset)
    if status:
        stmt = select(Notification).where(Notification.status == status).order_by(Notification.created_at.desc()).limit(limit).offset(offset)
    items = list(session.scalars(stmt))
    return NotificationListResponse(items=[NotificationRead.model_validate(item) for item in items], limit=limit, offset=offset)
