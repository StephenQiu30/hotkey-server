from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Query

from api.dependencies import Authenticated, Notifications
from api.responses import READ_ERROR_CODES, WRITE_ERROR_CODES, error_responses
from notifications.schemas import NotificationPage, NotificationView

router = APIRouter(prefix="/notifications", tags=["notifications"])


@router.get(
    "",
    response_model=NotificationPage,
    responses=error_responses(*READ_ERROR_CODES),
    operation_id="listNotifications",
)
def notifications(
    service: Notifications,
    owner: Authenticated,
    limit: Annotated[int, Query(ge=1, le=100)] = 20,
    cursor: Annotated[str | None, Query(max_length=128)] = None,
    unread_only: bool = False,
) -> NotificationPage:
    return service.notifications(limit=limit, cursor=cursor, unread_only=unread_only)


@router.post(
    "/{identity}/read",
    response_model=NotificationView,
    responses=error_responses(*WRITE_ERROR_CODES, 404),
    operation_id="markNotificationRead",
)
def mark_notification_read(
    identity: UUID, service: Notifications, owner: Authenticated
) -> NotificationView:
    return service.mark_read(identity)
