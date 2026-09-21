from __future__ import annotations

import json
import re
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4, uuid5

from sqlalchemy import select, update
from sqlalchemy.orm import Session

from jobs.models import Job, JobAttempt, ProcessedMessage
from jobs.schemas import JobStatus

type CheckpointValue = str | int | bool | None
type Clock = Callable[[], datetime]

_SCHEDULE_NAMESPACE = UUID("3e9db697-785d-4d82-a152-66dd5266ee9a")
_RESOURCE_ATTEMPT_NAMESPACE = UUID("f41e16e5-70e2-47fb-b5db-b216f4c03c4e")
_MAX_CHECKPOINT_ITEMS = 32
_STABLE_KEY_PATTERN = re.compile(r"^[a-z][a-z0-9_.:-]{0,127}$")


class JobExecutionError(RuntimeError):
    """Base class for recoverable job execution conflicts."""


class JobLeaseUnavailableError(JobExecutionError):
    """The job is terminal or currently owned by a live executor."""


class StaleExecutionLeaseError(JobExecutionError):
    """The caller no longer owns the current execution epoch."""


class CheckpointConflictError(JobExecutionError):
    """A checkpoint attempted to skip or rewrite durable progress."""


@dataclass(frozen=True, slots=True)
class MessageReference:
    message_id: UUID
    topic: str
    partition: int
    offset: int

    def __post_init__(self) -> None:
        if not self.topic or self.partition < 0 or self.offset < 0:
            raise ValueError("message reference must contain a valid topic and position")


@dataclass(frozen=True, slots=True)
class ExecutionLease:
    job_id: UUID
    worker_id: str
    epoch: int
    expires_at: datetime
    checkpoint_sequence: int
    checkpoint: dict[str, CheckpointValue]


@dataclass(frozen=True, slots=True)
class ScheduleWindow:
    start: datetime
    end: datetime

    def __post_init__(self) -> None:
        if self.start.tzinfo is None or self.end.tzinfo is None or self.end <= self.start:
            raise ValueError("schedule windows must be ordered timezone-aware datetimes")


@dataclass(frozen=True, slots=True)
class CatchupPlan:
    windows: tuple[ScheduleWindow, ...]
    skipped_windows: int


def _utc_text(value: datetime) -> str:
    return value.astimezone(UTC).isoformat().replace("+00:00", "Z")


def plan_catchup_windows(
    *,
    due_from: datetime,
    due_until: datetime,
    cadence: timedelta,
    max_windows: int,
) -> CatchupPlan:
    if due_from.tzinfo is None or due_until.tzinfo is None:
        raise ValueError("catchup bounds must be timezone-aware")
    if due_until < due_from:
        raise ValueError("catchup end cannot precede start")
    if cadence <= timedelta(0):
        raise ValueError("catchup cadence must be positive")
    if not 1 <= max_windows <= 100:
        raise ValueError("max_windows must be between 1 and 100")

    total_windows = int((due_until - due_from) // cadence)
    skipped_windows = max(0, total_windows - max_windows)
    windows = tuple(
        ScheduleWindow(
            start=due_from + cadence * index,
            end=due_from + cadence * (index + 1),
        )
        for index in range(skipped_windows, total_windows)
    )
    return CatchupPlan(windows=windows, skipped_windows=skipped_windows)


def scheduled_operation_id(
    owner_id: UUID,
    kind: str,
    schedule_key: str,
    window: ScheduleWindow,
) -> UUID:
    if _STABLE_KEY_PATTERN.fullmatch(schedule_key) is None:
        raise ValueError("schedule_key must be a stable lowercase identifier")
    identity = json.dumps(
        [str(owner_id), kind, schedule_key, _utc_text(window.start), _utc_text(window.end)],
        ensure_ascii=True,
        separators=(",", ":"),
    )
    return uuid5(_SCHEDULE_NAMESPACE, identity)


def resource_attempt_id(
    *,
    operation_id: UUID,
    component_key: str,
    stage: str,
    sequence: int,
) -> UUID:
    if _STABLE_KEY_PATTERN.fullmatch(component_key) is None:
        raise ValueError("component_key must be a stable lowercase identifier")
    if _STABLE_KEY_PATTERN.fullmatch(stage) is None:
        raise ValueError("stage must be a stable lowercase identifier")
    if sequence < 1:
        raise ValueError("sequence must be positive")
    identity = json.dumps(
        [str(operation_id), component_key, stage, sequence],
        ensure_ascii=True,
        separators=(",", ":"),
    )
    return uuid5(_RESOURCE_ATTEMPT_NAMESPACE, identity)


def _validate_checkpoint(checkpoint: Mapping[str, CheckpointValue]) -> dict[str, CheckpointValue]:
    if len(checkpoint) > _MAX_CHECKPOINT_ITEMS:
        raise ValueError(f"checkpoint cannot contain more than {_MAX_CHECKPOINT_ITEMS} items")
    if any(_STABLE_KEY_PATTERN.fullmatch(key) is None for key in checkpoint):
        raise ValueError("checkpoint keys must be stable lowercase identifiers")
    if any(
        item is not None and not isinstance(item, (str, int, bool)) for item in checkpoint.values()
    ):
        raise ValueError("checkpoint values must be scalar")
    return dict(checkpoint)


class JobExecutionService:
    def __init__(
        self,
        session: Session,
        *,
        lease_seconds: int,
        clock: Clock | None = None,
    ) -> None:
        if not 5 <= lease_seconds <= 300:
            raise ValueError("lease_seconds must be between 5 and 300")
        self._session = session
        self._lease_duration = timedelta(seconds=lease_seconds)
        self._clock = clock or (lambda: datetime.now(UTC))

    def acquire(self, *, job_id: UUID, worker_id: str) -> ExecutionLease:
        if not worker_id or len(worker_id) > 128:
            raise ValueError("worker_id must contain between 1 and 128 characters")
        now = self._clock()
        expires_at = now + self._lease_duration
        self._session.rollback()
        with self._session.begin():
            model = self._lock_job(job_id)
            if model.status == JobStatus.RUNNING.value:
                if model.lease_expires_at is None or model.lease_expires_at > now:
                    raise JobLeaseUnavailableError("job already has a live execution lease")
                self._session.execute(
                    update(JobAttempt)
                    .where(
                        JobAttempt.job_id == model.id,
                        JobAttempt.lease_epoch == model.lease_epoch,
                        JobAttempt.finished_at.is_(None),
                    )
                    .values(finished_at=now, outcome="expired")
                )
            elif model.status != JobStatus.QUEUED.value:
                raise JobLeaseUnavailableError("terminal jobs cannot be acquired")

            model.status = JobStatus.RUNNING.value
            model.lease_owner = worker_id
            model.lease_epoch += 1
            model.lease_expires_at = expires_at
            model.started_at = model.started_at or now
            model.updated_at = now
            self._session.add(
                JobAttempt(
                    id=uuid4(),
                    job_id=model.id,
                    lease_epoch=model.lease_epoch,
                    worker_id=worker_id,
                    started_at=now,
                    lease_expires_at=expires_at,
                    finished_at=None,
                    outcome=None,
                )
            )
            lease = self._lease(model)
        return lease

    def save_checkpoint(
        self,
        lease: ExecutionLease,
        *,
        sequence: int,
        checkpoint: Mapping[str, CheckpointValue],
    ) -> ExecutionLease:
        stored_checkpoint = _validate_checkpoint(checkpoint)
        now = self._clock()
        expires_at = now + self._lease_duration
        self._session.rollback()
        with self._session.begin():
            model = self._lock_job(lease.job_id)
            self._require_current_lease(model, lease, now)
            if sequence == model.checkpoint_sequence:
                if model.checkpoint != stored_checkpoint:
                    raise CheckpointConflictError("checkpoint sequence already has other data")
            elif sequence == model.checkpoint_sequence + 1:
                model.checkpoint_sequence = sequence
                model.checkpoint = stored_checkpoint
            else:
                raise CheckpointConflictError("checkpoint sequence must advance by one")

            model.lease_expires_at = expires_at
            model.updated_at = now
            self._session.execute(
                update(JobAttempt)
                .where(
                    JobAttempt.job_id == model.id,
                    JobAttempt.lease_epoch == model.lease_epoch,
                    JobAttempt.finished_at.is_(None),
                )
                .values(lease_expires_at=expires_at)
            )
            renewed = self._lease(model)
        return renewed

    def complete(self, lease: ExecutionLease, *, message: MessageReference) -> None:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            model = self._lock_job(lease.job_id)
            self._require_current_lease(model, lease, now)
            model.status = JobStatus.SUCCEEDED.value
            model.lease_owner = None
            model.lease_expires_at = None
            model.completed_at = now
            model.updated_at = now
            self._session.execute(
                update(JobAttempt)
                .where(
                    JobAttempt.job_id == model.id,
                    JobAttempt.lease_epoch == model.lease_epoch,
                    JobAttempt.finished_at.is_(None),
                )
                .values(finished_at=now, outcome="succeeded")
            )
            self._session.add(
                ProcessedMessage(
                    id=message.message_id,
                    job_id=model.id,
                    topic=message.topic,
                    partition=message.partition,
                    message_offset=message.offset,
                    processed_at=now,
                )
            )

    def is_processed(self, message_id: UUID) -> bool:
        processed = self._session.get(ProcessedMessage, message_id) is not None
        self._session.rollback()
        return processed

    def _lock_job(self, job_id: UUID) -> Job:
        model = self._session.scalar(select(Job).where(Job.id == job_id).with_for_update())
        if model is None:
            raise JobLeaseUnavailableError("job does not exist")
        return model

    @staticmethod
    def _require_current_lease(model: Job, lease: ExecutionLease, now: datetime) -> None:
        if (
            model.status != JobStatus.RUNNING.value
            or model.lease_owner != lease.worker_id
            or model.lease_epoch != lease.epoch
            or model.lease_expires_at is None
            or model.lease_expires_at <= now
        ):
            raise StaleExecutionLeaseError("execution lease is stale or expired")

    @staticmethod
    def _lease(model: Job) -> ExecutionLease:
        if model.lease_owner is None or model.lease_expires_at is None:
            raise RuntimeError("running job is missing its execution lease")
        return ExecutionLease(
            job_id=model.id,
            worker_id=model.lease_owner,
            epoch=model.lease_epoch,
            expires_at=model.lease_expires_at,
            checkpoint_sequence=model.checkpoint_sequence,
            checkpoint=dict(model.checkpoint),
        )
