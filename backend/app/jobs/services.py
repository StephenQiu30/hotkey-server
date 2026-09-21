from __future__ import annotations

import hashlib
import json
from datetime import UTC, datetime
from uuid import UUID, uuid4

from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session

from core.errors import ApplicationError
from jobs.models import Job, OutboxMessage
from jobs.schemas import JobAcceptanceInput, JobStatus, JobView

JOB_ACCEPTED_EVENT_TYPE = "job.accepted.v1"
JOB_ACCEPTED_TOPIC = "hotkey.jobs.accepted.v1"


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
