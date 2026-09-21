from __future__ import annotations

import hashlib
import json
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID, uuid4

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session

from core.errors import ApplicationError
from jobs.execution import ScheduleWindow, scheduled_operation_id
from jobs.models import Job, OutboxMessage
from jobs.schemas import JobAcceptanceInput, JobStatus, JobView

JOB_ACCEPTED_EVENT_TYPE = "job.accepted.v1"
JOB_ACCEPTED_TOPIC = "hotkey.jobs.accepted.v1"

type PublishOutbox = Callable[["OutboxEnvelope"], None]


@dataclass(frozen=True, slots=True)
class OutboxEnvelope:
    message_id: UUID
    topic: str
    message_key: UUID
    event_type: str
    payload: dict[str, str]

    def message_body(self) -> dict[str, str | int]:
        return {
            **self.payload,
            "schema_version": 1,
            "message_id": str(self.message_id),
            "event_type": self.event_type,
        }


def fingerprint_request(command: JobAcceptanceInput) -> bytes:
    canonical = json.dumps(
        {"kind": command.kind, "scope": command.scope},
        ensure_ascii=False,
        allow_nan=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return hashlib.sha256(canonical.encode()).digest()


class JobService:
    def __init__(self, session: Session) -> None:
        self._session = session

    def accept(self, *, owner_id: UUID, command: JobAcceptanceInput) -> JobView:
        fingerprint = fingerprint_request(command)
        now = datetime.now(UTC)
        job_id = uuid4()
        self._session.rollback()

        with self._session.begin():
            inserted_id = self._session.scalar(
                insert(Job)
                .values(
                    id=job_id,
                    owner_id=owner_id,
                    operation_id=command.operation_id,
                    kind=command.kind,
                    scope=command.scope,
                    request_fingerprint=fingerprint,
                    status=JobStatus.QUEUED.value,
                    created_at=now,
                    updated_at=now,
                )
                .on_conflict_do_nothing(constraint="jobs_owner_kind_operation_key")
                .returning(Job.id)
            )

            if inserted_id is not None:
                self._session.add(
                    OutboxMessage(
                        id=uuid4(),
                        aggregate_id=job_id,
                        topic=JOB_ACCEPTED_TOPIC,
                        message_key=job_id,
                        event_type=JOB_ACCEPTED_EVENT_TYPE,
                        payload={
                            "job_id": str(job_id),
                            "owner_id": str(owner_id),
                            "operation_id": str(command.operation_id),
                            "kind": command.kind,
                        },
                        created_at=now,
                        published_at=None,
                    )
                )
                view = JobView(
                    id=job_id,
                    owner_id=owner_id,
                    operation_id=command.operation_id,
                    kind=command.kind,
                    status=JobStatus.QUEUED,
                    created_at=now,
                )
            else:
                existing = self._session.scalar(
                    select(Job).where(
                        Job.owner_id == owner_id,
                        Job.kind == command.kind,
                        Job.operation_id == command.operation_id,
                    )
                )
                if existing is None:
                    raise RuntimeError("conflicting job is not visible after insert conflict")
                if existing.request_fingerprint != fingerprint:
                    raise ApplicationError("idempotency_conflict")
                view = self._view(existing)

        return view

    def accept_schedule_window(
        self,
        *,
        owner_id: UUID,
        kind: str,
        schedule_key: str,
        window: ScheduleWindow,
        scope: Mapping[str, str | int | bool | None],
    ) -> JobView:
        reserved = {"schedule_key", "window_start", "window_end"}
        if reserved.intersection(scope):
            raise ValueError("scope cannot replace reserved schedule fields")
        scheduled_scope = {
            **scope,
            "schedule_key": schedule_key,
            "window_start": window.start.astimezone(UTC).isoformat().replace("+00:00", "Z"),
            "window_end": window.end.astimezone(UTC).isoformat().replace("+00:00", "Z"),
        }
        return self.accept(
            owner_id=owner_id,
            command=JobAcceptanceInput(
                operation_id=scheduled_operation_id(
                    owner_id,
                    kind,
                    schedule_key,
                    window,
                ),
                kind=kind,
                scope=scheduled_scope,
            ),
        )

    @staticmethod
    def _view(model: Job) -> JobView:
        return JobView(
            id=model.id,
            owner_id=model.owner_id,
            operation_id=model.operation_id,
            kind=model.kind,
            status=JobStatus(model.status),
            created_at=model.created_at,
        )


class OutboxService:
    def __init__(self, session: Session) -> None:
        self._session = session

    def publish_pending(
        self,
        publish: PublishOutbox,
        *,
        batch_size: int = 25,
        published_at: datetime | None = None,
    ) -> int:
        if not 1 <= batch_size <= 100:
            raise ValueError("batch_size must be between 1 and 100")
        now = published_at or datetime.now(UTC)
        self._session.rollback()
        with self._session.begin():
            messages = list(
                self._session.scalars(
                    select(OutboxMessage)
                    .where(OutboxMessage.published_at.is_(None))
                    .order_by(OutboxMessage.created_at, OutboxMessage.id)
                    .limit(batch_size)
                    .with_for_update(skip_locked=True)
                )
            )
            for model in messages:
                publish(
                    OutboxEnvelope(
                        message_id=model.id,
                        topic=model.topic,
                        message_key=model.message_key,
                        event_type=model.event_type,
                        payload=dict(model.payload),
                    )
                )
                model.published_at = now
        return len(messages)
