from base64 import b64decode, urlsafe_b64encode
from binascii import Error as Base64Error
from datetime import datetime
from typing import Literal
from uuid import UUID, uuid4

from sqlalchemy import func, or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from audit.services import audit
from core.clock import utcnow
from core.errors import AppError
from notifications.models import Notification
from notifications.schemas import NotificationPage, NotificationView

EventChangeType = Literal[
    "add_member", "remove_member", "merge_in", "merge_out", "split_in", "split_out"
]

CHANGE_DETAILS: dict[EventChangeType, tuple[str, str]] = {
    "add_member": ("event_member_added", "事件新增了一条内容"),
    "remove_member": ("event_member_removed", "事件移出了一条内容"),
    "merge_in": ("event_merged_in", "其他事件已合并到当前事件"),
    "merge_out": ("event_merged_out", "当前事件已合并并归档"),
    "split_in": ("event_split_in", "当前事件由另一事件拆分创建"),
    "split_out": ("event_split_out", "当前事件已有部分内容拆出"),
}


def record_event_change(
    session: Session,
    *,
    event_id: UUID,
    change_id: UUID,
    change_type: EventChangeType,
    created_at: datetime,
) -> None:
    kind, message = CHANGE_DETAILS[change_type]
    session.execute(
        insert(Notification)
        .values(
            id=uuid4(),
            event_id=event_id,
            change_id=change_id,
            rule_version=1,
            kind=kind,
            message=message,
            created_at=created_at,
            read_at=None,
        )
        .on_conflict_do_nothing(
            index_elements=[
                Notification.rule_version,
                Notification.event_id,
                Notification.change_id,
            ]
        )
    )


def _encode_cursor(identity: UUID) -> str:
    return urlsafe_b64encode(identity.bytes).decode("ascii").rstrip("=")


def _decode_cursor(value: str) -> UUID:
    try:
        raw = b64decode(value + "==", altchars=b"-_", validate=True)
        identity = UUID(bytes=raw)
    except (Base64Error, ValueError) as error:
        raise AppError("invalid_cursor", 422) from error
    if _encode_cursor(identity) != value:
        raise AppError("invalid_cursor", 422)
    return identity


class NotificationService:
    def __init__(self, factory: sessionmaker[Session]):
        self.factory = factory

    @staticmethod
    def _view(notification: Notification) -> NotificationView:
        return NotificationView.model_validate(notification, from_attributes=True)

    def notifications(
        self, *, limit: int, cursor: str | None, unread_only: bool
    ) -> NotificationPage:
        with self.factory() as session:
            query = select(Notification)
            if unread_only:
                query = query.where(Notification.read_at.is_(None))
            if cursor is not None:
                anchor_id = _decode_cursor(cursor)
                anchor = session.get(Notification, anchor_id)
                if anchor is None:
                    raise AppError("invalid_cursor", 422)
                query = query.where(
                    or_(
                        Notification.created_at < anchor.created_at,
                        (
                            (Notification.created_at == anchor.created_at)
                            & (Notification.id < anchor.id)
                        ),
                    )
                )
            rows = list(
                session.scalars(
                    query.order_by(Notification.created_at.desc(), Notification.id.desc()).limit(
                        limit + 1
                    )
                )
            )
            visible = rows[:limit]
            unread_count = session.scalar(
                select(func.count()).select_from(Notification).where(Notification.read_at.is_(None))
            )
            return NotificationPage(
                items=[self._view(item) for item in visible],
                next_cursor=_encode_cursor(visible[-1].id) if len(rows) > limit else None,
                unread_count=unread_count or 0,
            )

    def mark_read(self, identity: UUID) -> NotificationView:
        with self.factory.begin() as session:
            notification = session.scalar(
                select(Notification).where(Notification.id == identity).with_for_update()
            )
            if notification is None:
                raise AppError("notification_not_found", 404)
            if notification.read_at is None:
                notification.read_at = utcnow()
                audit(session, "notification_read", str(identity))
            return self._view(notification)
