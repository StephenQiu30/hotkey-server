from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import CheckConstraint, ForeignKey, LargeBinary, String, UniqueConstraint, text
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base

type JsonValue = str | int | bool | None


class Job(Base):
    __tablename__ = "jobs"
    __table_args__ = (
        UniqueConstraint(
            "owner_id",
            "kind",
            "operation_id",
            name="jobs_owner_kind_operation_key",
        ),
        CheckConstraint(
            "kind ~ '^[a-z][a-z0-9_.-]{0,63}$'",
            name="jobs_kind_check",
        ),
        CheckConstraint(
            "status IN ('queued', 'running', 'succeeded', "
            "'partially_succeeded', 'failed', 'cancelled')",
            name="jobs_status_check",
        ),
        CheckConstraint(
            "octet_length(request_fingerprint) = 32",
            name="jobs_request_fingerprint_length_check",
        ),
        CheckConstraint("jsonb_typeof(scope) = 'object'", name="jobs_scope_object_check"),
        CheckConstraint("updated_at >= created_at", name="jobs_updated_at_check"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    operation_id: Mapped[UUID]
    kind: Mapped[str] = mapped_column(String(64))
    scope: Mapped[dict[str, JsonValue]] = mapped_column(JSONB)
    request_fingerprint: Mapped[bytes] = mapped_column(LargeBinary(32))
    status: Mapped[str] = mapped_column(String(32), server_default=text("'queued'"))
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]


class OutboxMessage(Base):
    __tablename__ = "outbox_messages"
    __table_args__ = (
        UniqueConstraint(
            "event_type",
            "aggregate_id",
            name="outbox_messages_event_aggregate_key",
        ),
        CheckConstraint(
            "jsonb_typeof(payload) = 'object'",
            name="outbox_messages_payload_object_check",
        ),
        CheckConstraint(
            "published_at IS NULL OR published_at >= created_at",
            name="outbox_messages_published_at_check",
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    aggregate_id: Mapped[UUID] = mapped_column(ForeignKey("jobs.id", ondelete="CASCADE"))
    topic: Mapped[str] = mapped_column(String(128))
    message_key: Mapped[UUID]
    event_type: Mapped[str] = mapped_column(String(64))
    payload: Mapped[dict[str, str]] = mapped_column(JSONB)
    created_at: Mapped[datetime]
    published_at: Mapped[datetime | None]
