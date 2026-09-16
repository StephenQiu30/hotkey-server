from base64 import b64decode, urlsafe_b64encode
from binascii import Error as Base64Error
from dataclasses import dataclass
from datetime import datetime
from typing import Literal, cast
from uuid import UUID, uuid4

from sqlalchemy import func, or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from audit.services import audit
from core.clock import utcnow
from core.errors import AppError
from notifications.models import Notification, TrendAlertOccurrence, TrendAlertRule
from notifications.schemas import (
    NotificationPage,
    NotificationView,
    TrendAlertMetric,
    TrendAlertRuleInput,
    TrendAlertRuleUpdate,
    TrendAlertRuleView,
)

EventChangeType = Literal[
    "add_member", "remove_member", "merge_in", "merge_out", "split_in", "split_out"
]


@dataclass(frozen=True)
class TrendAlertRuleSnapshot:
    id: UUID
    event_id: UUID
    source: str
    metric: TrendAlertMetric
    bucket_hours: Literal[1, 6, 24]
    threshold_count: int
    version: int


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
            trend_occurrence_id=None,
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


def _rule_view(rule: TrendAlertRule) -> TrendAlertRuleView:
    return TrendAlertRuleView.model_validate(rule, from_attributes=True)


def list_trend_alert_rules(session: Session, event_id: UUID) -> list[TrendAlertRuleView]:
    rules = session.scalars(
        select(TrendAlertRule)
        .where(TrendAlertRule.event_id == event_id)
        .order_by(TrendAlertRule.created_at, TrendAlertRule.id)
    )
    return [_rule_view(rule) for rule in rules]


def create_trend_alert_rule(
    session: Session,
    *,
    event_id: UUID,
    data: TrendAlertRuleInput,
    created_at: datetime,
) -> TrendAlertRuleView:
    identity = session.scalar(
        insert(TrendAlertRule)
        .values(
            id=uuid4(),
            event_id=event_id,
            source=data.source,
            metric=data.metric,
            bucket_hours=data.bucket_hours,
            threshold_count=data.threshold_count,
            version=1,
            enabled=True,
            created_at=created_at,
            updated_at=created_at,
        )
        .on_conflict_do_nothing(
            index_elements=[
                TrendAlertRule.event_id,
                TrendAlertRule.source,
                TrendAlertRule.metric,
                TrendAlertRule.bucket_hours,
            ]
        )
        .returning(TrendAlertRule.id)
    )
    if identity is None:
        raise AppError("trend_alert_rule_exists", 409)
    rule = session.get(TrendAlertRule, identity)
    assert rule is not None
    audit(session, "trend_alert_rule_created", str(rule.id))
    return _rule_view(rule)


def update_trend_alert_rule(
    session: Session,
    *,
    event_id: UUID,
    rule_id: UUID,
    data: TrendAlertRuleUpdate,
    updated_at: datetime,
) -> TrendAlertRuleView:
    rule = session.scalar(
        select(TrendAlertRule)
        .where(TrendAlertRule.id == rule_id, TrendAlertRule.event_id == event_id)
        .with_for_update()
    )
    if rule is None:
        raise AppError("trend_alert_rule_not_found", 404)
    if rule.version != data.expected_version:
        raise AppError("trend_alert_rule_version_conflict", 409)
    if rule.threshold_count != data.threshold_count or rule.enabled != data.enabled:
        rule.threshold_count = data.threshold_count
        rule.enabled = data.enabled
        rule.version += 1
        rule.updated_at = updated_at
        audit(session, "trend_alert_rule_updated", str(rule.id))
    return _rule_view(rule)


def active_trend_alert_rules(
    session: Session, event_id: UUID, *, lock: bool = False
) -> list[TrendAlertRuleSnapshot]:
    query = (
        select(TrendAlertRule)
        .where(TrendAlertRule.event_id == event_id, TrendAlertRule.enabled.is_(True))
        .order_by(TrendAlertRule.id)
    )
    rules = session.scalars(query.with_for_update() if lock else query)
    return [
        TrendAlertRuleSnapshot(
            id=rule.id,
            event_id=rule.event_id,
            source=rule.source,
            metric=cast(TrendAlertMetric, rule.metric),
            bucket_hours=cast(Literal[1, 6, 24], rule.bucket_hours),
            threshold_count=rule.threshold_count,
            version=rule.version,
        )
        for rule in rules
    ]


def active_trend_alert_event_ids(session: Session) -> list[UUID]:
    return list(
        session.scalars(
            select(TrendAlertRule.event_id)
            .where(TrendAlertRule.enabled.is_(True))
            .distinct()
            .order_by(TrendAlertRule.event_id)
        )
    )


def record_trend_alert(
    session: Session,
    *,
    rule: TrendAlertRuleSnapshot,
    bucket_start: datetime,
    bucket_end: datetime,
    metric_value: int,
    created_at: datetime,
) -> bool:
    occurrence_id = session.scalar(
        insert(TrendAlertOccurrence)
        .values(
            id=uuid4(),
            rule_id=rule.id,
            event_id=rule.event_id,
            rule_version=rule.version,
            bucket_start=bucket_start,
            bucket_end=bucket_end,
            metric_value=metric_value,
            created_at=created_at,
        )
        .on_conflict_do_nothing(
            index_elements=[
                TrendAlertOccurrence.rule_id,
                TrendAlertOccurrence.rule_version,
                TrendAlertOccurrence.bucket_start,
            ]
        )
        .returning(TrendAlertOccurrence.id)
    )
    if occurrence_id is None:
        return False
    metric_labels = {
        "new_posts": "新增根帖",
        "new_discussions": "新增评论/回复",
        "observed_reply_delta": "已观察回复增量",
    }
    session.add(
        Notification(
            id=uuid4(),
            event_id=rule.event_id,
            change_id=None,
            trend_occurrence_id=occurrence_id,
            rule_version=rule.version,
            kind="trend_threshold_reached",
            message=(
                f"{rule.source} {rule.bucket_hours}小时桶{metric_labels[rule.metric]}"
                f"达到 {metric_value}（阈值 {rule.threshold_count}）"
            ),
            created_at=created_at,
            read_at=None,
        )
    )
    audit(session, "trend_alert_triggered", str(occurrence_id))
    return True


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
