from __future__ import annotations

from datetime import UTC, datetime
from uuid import UUID, uuid4, uuid5

from sqlalchemy import select, text
from sqlalchemy.orm import Session

from core.config import Settings
from jobs.schemas import JobAcceptanceInput, JobObservationContext
from jobs.services import JobService
from notifications.models import NotificationDelivery, NotificationTarget
from notifications.schemas import DeliveryStatus, NotificationChannel, TargetInput, TargetView

DELIVERY_OPERATION_NAMESPACE = UUID("8acccf68-f7a3-4f27-bc8c-70d41a20b175")


def delivery_operation_id(*, report_id: UUID, version: int, target_id: UUID) -> UUID:
    return uuid5(DELIVERY_OPERATION_NAMESPACE, f"{report_id}:{version}:{target_id}")


class NotificationTargetService:
    def __init__(self, session: Session) -> None:
        self._session = session

    def add_in_transaction(
        self, *, owner_id: UUID, target: TargetInput, now: datetime
    ) -> TargetView:
        if not self._session.in_transaction():
            raise RuntimeError("target creation requires the caller's transaction")
        if target.channel is NotificationChannel.FEISHU and target.recipients:
            raise ValueError("Feishu webhook targets have no recipients")
        if (
            self._session.scalar(
                select(NotificationTarget.id).where(
                    NotificationTarget.owner_id == owner_id, NotificationTarget.name == target.name
                )
            )
            is not None
        ):
            raise ValueError("notification target name already exists")
        model = NotificationTarget(
            id=uuid4(),
            owner_id=owner_id,
            name=target.name,
            channel=target.channel.value,
            recipients=list(target.recipients),
            secret_env=target.secret_env,
            enabled=True,
            created_at=now,
            updated_at=now,
        )
        self._session.add(model)
        self._session.flush()
        return self._view(model)

    def list(self, *, owner_id: UUID) -> tuple[TargetView, ...]:
        rows = self._session.scalars(
            select(NotificationTarget)
            .where(NotificationTarget.owner_id == owner_id)
            .order_by(NotificationTarget.name)
        ).all()
        return tuple(self._view(row) for row in rows)

    @staticmethod
    def _view(model: NotificationTarget) -> TargetView:
        return TargetView(
            id=model.id,
            name=model.name,
            channel=NotificationChannel(model.channel),
            recipients=tuple(model.recipients),
            secret_env=model.secret_env,
            enabled=model.enabled,
            created_at=model.created_at,
        )


class NotificationService:
    def __init__(self, session: Session, settings: Settings) -> None:
        self._session = session
        self._settings = settings

    def enqueue_due_in_transaction(self, *, now: datetime) -> int:
        if not self._settings.notifications_enabled:
            return 0
        if not self._session.in_transaction():
            raise RuntimeError("notification scan requires the caller's transaction")
        if now.tzinfo is None:
            raise ValueError("notification scan time must be timezone-aware")
        # Only the first final version of each daily report window is sent. A later
        # regeneration must never silently push an updated report to old targets.
        rows = self._session.execute(
            text("""
            WITH first_final AS (
                SELECT r.*, row_number() OVER (
                    PARTITION BY r.owner_id, r.topic_id, r.kind, r.window_start
                    ORDER BY r.version ASC
                ) AS final_rank
                FROM reports r WHERE r.kind = 'daily' AND r.status = 'final'
            )
            SELECT r.id AS report_id, r.owner_id, r.version, t.id AS target_id
            FROM first_final r
            JOIN monitor_topics topic ON topic.id = r.topic_id AND topic.owner_id = r.owner_id
            JOIN notification_targets t ON t.owner_id = r.owner_id
                AND t.name IN (SELECT jsonb_array_elements_text(topic.notification_target_names))
            WHERE r.final_rank = 1 AND t.enabled AND t.channel = 'feishu'
              AND NOT EXISTS (
                  SELECT 1 FROM notification_deliveries d
                  WHERE d.report_id = r.id AND d.report_version = r.version
                    AND d.target_id = t.id
              )
            ORDER BY r.owner_id, r.id, t.id
            LIMIT 100
        """)
        ).mappings()
        accepted = 0
        for row in rows:
            delivery_id = self._session.execute(
                text("""
                INSERT INTO notification_deliveries
                    (id, owner_id, report_id, report_version, target_id, status,
                     attempt_count, created_at, updated_at)
                VALUES (:id, :owner_id, :report_id, :version, :target_id, 'pending',
                        0, :now, :now)
                ON CONFLICT (report_id, report_version, target_id) DO NOTHING
                RETURNING id
            """),
                {
                    "id": uuid4(),
                    "owner_id": row["owner_id"],
                    "report_id": row["report_id"],
                    "version": row["version"],
                    "target_id": row["target_id"],
                    "now": now,
                },
            ).scalar_one_or_none()
            if delivery_id is None:
                continue
            JobService(self._session, clock=lambda: now).accept_in_transaction(
                owner_id=row["owner_id"],
                command=JobAcceptanceInput(
                    operation_id=delivery_operation_id(
                        report_id=row["report_id"],
                        version=row["version"],
                        target_id=row["target_id"],
                    ),
                    kind="notification.send",
                    observation=JobObservationContext(
                        configuration_ref=f"report:{row['report_id']}",
                        configuration_version=row["version"],
                    ),
                    scope={"delivery_id": str(delivery_id)},
                ),
            )
            accepted += 1
        return accepted

    def begin_sending(
        self, *, delivery_id: UUID, owner_id: UUID, now: datetime
    ) -> tuple[DeliveryStatus, NotificationDelivery | None]:
        with self._session.begin():
            delivery = self._session.scalar(
                select(NotificationDelivery)
                .where(
                    NotificationDelivery.id == delivery_id,
                    NotificationDelivery.owner_id == owner_id,
                )
                .with_for_update()
            )
            if delivery is None:
                raise ValueError("notification delivery does not exist")
            status = DeliveryStatus(delivery.status)
            if status is DeliveryStatus.SENDING:
                delivery.status = DeliveryStatus.UNKNOWN.value
                delivery.last_error_code = "interrupted_after_sending"
                delivery.updated_at = now
                return DeliveryStatus.UNKNOWN, None
            if (
                status in {DeliveryStatus.SUCCEEDED, DeliveryStatus.UNKNOWN}
                or delivery.attempt_count >= 3
            ):
                return status, None
            delivery.status = DeliveryStatus.SENDING.value
            delivery.attempt_count += 1
            delivery.last_error_code = None
            delivery.updated_at = now
            self._session.flush()
            return DeliveryStatus.SENDING, delivery

    def finish(
        self, *, delivery_id: UUID, status: DeliveryStatus, error_code: str | None, now: datetime
    ) -> None:
        if status not in {DeliveryStatus.SUCCEEDED, DeliveryStatus.FAILED, DeliveryStatus.UNKNOWN}:
            raise ValueError("invalid delivery terminal status")
        with self._session.begin():
            delivery = self._session.scalar(
                select(NotificationDelivery)
                .where(NotificationDelivery.id == delivery_id)
                .with_for_update()
            )
            if delivery is None or delivery.status != DeliveryStatus.SENDING.value:
                return
            delivery.status = status.value
            delivery.last_error_code = error_code
            delivery.sent_at = now if status is DeliveryStatus.SUCCEEDED else None
            delivery.updated_at = now


def mark_interrupted_sending_in_transaction(
    session: Session, *, delivery_id: UUID, now: datetime
) -> None:
    if not session.in_transaction():
        raise RuntimeError("notification recovery requires caller transaction")
    session.execute(
        text("""
        UPDATE notification_deliveries SET status = 'unknown',
            last_error_code = 'interrupted_after_sending', updated_at = :now
        WHERE id = :delivery_id AND status = 'sending'
    """),
        {"delivery_id": delivery_id, "now": now.astimezone(UTC)},
    )
