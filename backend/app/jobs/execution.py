from __future__ import annotations

import json
import re
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4, uuid5

from sqlalchemy import func, select, update
from sqlalchemy.orm import Session

from jobs.models import Job, JobAttempt, OutboxMessage, ProcessedMessage
from jobs.schemas import JobFailureCategory, JobStage, JobStatus

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
class JobExecutionFailure(Exception):  # noqa: N818 - frozen domain contract name
    error_code: str
    category: JobFailureCategory
    occurred_at: datetime
    next_action: str
    manual_retry_allowed: bool = False
    retry_at: datetime | None = None
    max_attempts: int | None = None

    def __post_init__(self) -> None:
        Exception.__init__(self, self.error_code)
        if _STABLE_KEY_PATTERN.fullmatch(self.error_code) is None:
            raise ValueError("error_code must be a stable lowercase identifier")
        if self.occurred_at.tzinfo is None:
            raise ValueError("occurred_at must be timezone-aware")
        if not self.next_action or len(self.next_action) > 512:
            raise ValueError("next_action must contain between 1 and 512 characters")
        automatic = self.category in {
            JobFailureCategory.TRANSIENT,
            JobFailureCategory.RATE_LIMITED,
        }
        complete_policy = self.retry_at is not None and self.max_attempts is not None
        if complete_policy:
            if self.retry_at is None or self.retry_at.tzinfo is None:
                raise ValueError("retry_at must be timezone-aware")
            if self.max_attempts is None or not 2 <= self.max_attempts <= 100:
                raise ValueError("max_attempts must be between 2 and 100")
        if not automatic and complete_policy:
            raise ValueError("permanent failures cannot carry an automatic retry policy")
        if (self.retry_at is None) != (self.max_attempts is None):
            raise ValueError("automatic retry requires retry_at and max_attempts together")


@dataclass(frozen=True, slots=True)
class JobCompletion:
    status: JobStatus
    failure: JobExecutionFailure | None = None

    def __post_init__(self) -> None:
        if self.status not in {JobStatus.SUCCEEDED, JobStatus.PARTIALLY_SUCCEEDED}:
            raise ValueError("completion status must be succeeded or partially_succeeded")
        if self.status is JobStatus.SUCCEEDED and self.failure is not None:
            raise ValueError("succeeded completion cannot contain a failure")
        if self.status is JobStatus.PARTIALLY_SUCCEEDED and self.failure is None:
            raise ValueError("partially_succeeded completion requires a failure")
        if self.failure is not None and (
            self.failure.retry_at is not None or self.failure.max_attempts is not None
        ):
            raise ValueError("partial completion cannot schedule an automatic retry")


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
class JobProgress:
    stage: JobStage
    items_saved: int

    def __post_init__(self) -> None:
        if self.items_saved < 0:
            raise ValueError("saved item count cannot be negative")


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
            elif model.next_run_at is not None and model.next_run_at > now:
                raise JobLeaseUnavailableError("job is not due for execution")

            model.status = JobStatus.RUNNING.value
            model.lease_owner = worker_id
            model.lease_epoch += 1
            model.lease_expires_at = expires_at
            model.started_at = model.started_at or now
            model.defer_reason = None
            model.next_run_at = None
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
        progress: JobProgress | None = None,
    ) -> ExecutionLease:
        self._session.rollback()
        with self._session.begin():
            return self.save_checkpoint_in_transaction(
                lease,
                sequence=sequence,
                checkpoint=checkpoint,
                progress=progress,
            )

    def require_current_lease_in_transaction(self, lease: ExecutionLease) -> ExecutionLease:
        """Lock and verify a live, non-cancelled lease in the caller's transaction."""
        now = self._clock()
        model = self._lock_job(lease.job_id)
        self._require_current_lease(model, lease, now)
        if model.cancel_requested_at is not None:
            raise JobLeaseUnavailableError("job cancellation has been requested")
        return self._lease(model)

    def require_current_lease_allowing_cancel_in_transaction(
        self, lease: ExecutionLease
    ) -> ExecutionLease:
        """Lock and verify a live lease when recording a cancellation gap."""
        now = self._clock()
        model = self._lock_job(lease.job_id)
        self._require_current_lease(model, lease, now)
        return self._lease(model)

    def require_current_operation_in_transaction(
        self,
        lease: ExecutionLease,
        *,
        owner_id: UUID,
        operation_id: UUID,
    ) -> ExecutionLease:
        """Verify that a live lease belongs to the expected owner and operation."""
        return self._lease(
            self._require_current_operation_model(
                lease, owner_id=owner_id, operation_id=operation_id
            )
        )

    def _require_current_operation_model(
        self,
        lease: ExecutionLease,
        *,
        owner_id: UUID,
        operation_id: UUID,
    ) -> Job:
        now = self._clock()
        model = self._lock_job(lease.job_id)
        self._require_current_lease(model, lease, now)
        if model.cancel_requested_at is not None:
            raise JobLeaseUnavailableError("job cancellation has been requested")
        if model.owner_id != owner_id or model.operation_id != operation_id:
            raise JobLeaseUnavailableError("job lease does not match the requested operation")
        return model

    def current_request_count_in_transaction(
        self,
        lease: ExecutionLease,
        *,
        owner_id: UUID,
        operation_id: UUID,
    ) -> int:
        """Return the fenced task-wide request count before another outbound attempt."""
        return self._require_current_operation_model(
            lease, owner_id=owner_id, operation_id=operation_id
        ).requests_sent

    def save_checkpoint_in_transaction(
        self,
        lease: ExecutionLease,
        *,
        sequence: int,
        checkpoint: Mapping[str, CheckpointValue],
        progress: JobProgress | None = None,
    ) -> ExecutionLease:
        """Persist one fenced checkpoint inside an existing outer transaction."""
        stored_checkpoint = _validate_checkpoint(checkpoint)
        now = self._clock()
        expires_at = now + self._lease_duration
        model = self._lock_job(lease.job_id)
        self._require_current_lease(model, lease, now)
        if sequence == model.checkpoint_sequence:
            if model.checkpoint != stored_checkpoint:
                raise CheckpointConflictError("checkpoint sequence already has other data")
            if progress is not None and (
                model.progress_stage != progress.stage.value
                or model.items_saved != progress.items_saved
            ):
                raise CheckpointConflictError("checkpoint sequence already has other progress")
        elif sequence == model.checkpoint_sequence + 1:
            if progress is not None and progress.items_saved < model.items_saved:
                raise CheckpointConflictError("saved item count cannot decrease")
            model.checkpoint_sequence = sequence
            model.checkpoint = stored_checkpoint
            if progress is not None:
                model.progress_stage = progress.stage.value
                model.items_saved = progress.items_saved
                model.progress_updated_at = now
        else:
            raise CheckpointConflictError("checkpoint sequence must advance by one")

        if model.cancel_deadline_at is not None:
            expires_at = min(expires_at, model.cancel_deadline_at)
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
        return self._lease(model)

    def begin_request(self, lease: ExecutionLease) -> tuple[ExecutionLease, bool]:
        self._session.rollback()
        with self._session.begin():
            return self.begin_request_in_transaction(lease)

    def begin_request_in_transaction(
        self,
        lease: ExecutionLease,
    ) -> tuple[ExecutionLease, bool]:
        """Fence and meter one outbound request inside an existing transaction."""
        now = self._clock()
        expires_at = now + self._lease_duration
        model = self._lock_job(lease.job_id)
        self._require_current_lease(model, lease, now)
        if model.cancel_requested_at is not None:
            return self._lease(model), False

        model.requests_sent += 1
        model.progress_stage = JobStage.REQUEST.value
        model.progress_updated_at = now
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
        return self._lease(model), True

    def cancellation_requested(self, lease: ExecutionLease) -> bool:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            model = self._lock_job(lease.job_id)
            self._require_current_lease(model, lease, now)
            return model.cancel_requested_at is not None

    def acknowledge_cancelled(
        self,
        *,
        job_id: UUID,
        message: MessageReference,
    ) -> bool:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            model = self._lock_job(job_id)
            if model.status != JobStatus.CANCELLED.value:
                return False
            processed = self._session.get(ProcessedMessage, message.message_id)
            if processed is not None:
                if processed.job_id != model.id:
                    raise RuntimeError("processed message belongs to another job")
                return True
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
        return True

    def complete(
        self,
        lease: ExecutionLease,
        *,
        message: MessageReference,
        completion: JobCompletion | None = None,
    ) -> None:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            self.complete_in_transaction(
                lease,
                message=message,
                completion=completion,
                now=now,
            )

    def complete_in_transaction(
        self,
        lease: ExecutionLease,
        *,
        message: MessageReference,
        completion: JobCompletion | None = None,
        now: datetime | None = None,
    ) -> None:
        """Persist terminal job state and inbox identity in the caller's transaction."""
        occurred_at = now or self._clock()
        resolved = completion or JobCompletion(status=JobStatus.SUCCEEDED)
        model = self._lock_job(lease.job_id)
        self._require_current_lease(model, lease, occurred_at)
        cancelled = model.cancel_requested_at is not None
        model.status = JobStatus.CANCELLED.value if cancelled else resolved.status.value
        model.lease_owner = None
        model.lease_expires_at = None
        model.completed_at = occurred_at
        if not cancelled and resolved.failure is None:
            model.last_error_code = None
            model.last_error_category = None
            model.last_error_at = None
            model.next_action = None
            model.manual_retry_allowed = False
        elif not cancelled:
            assert resolved.failure is not None
            model.last_error_code = resolved.failure.error_code
            model.last_error_category = resolved.failure.category.value
            model.last_error_at = resolved.failure.occurred_at
            model.next_action = resolved.failure.next_action
            model.manual_retry_allowed = resolved.failure.manual_retry_allowed
        model.updated_at = occurred_at
        self._session.execute(
            update(JobAttempt)
            .where(
                JobAttempt.job_id == model.id,
                JobAttempt.lease_epoch == model.lease_epoch,
                JobAttempt.finished_at.is_(None),
            )
            .values(
                finished_at=occurred_at,
                outcome="cancelled" if cancelled else "succeeded",
            )
        )
        self._session.add(
            ProcessedMessage(
                id=message.message_id,
                job_id=model.id,
                topic=message.topic,
                partition=message.partition,
                message_offset=message.offset,
                processed_at=occurred_at,
            )
        )

    def record_failure(
        self,
        lease: ExecutionLease,
        *,
        message: MessageReference,
        failure: JobExecutionFailure,
    ) -> None:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            self.record_failure_in_transaction(
                lease,
                message=message,
                failure=failure,
                now=now,
            )

    def record_failure_in_transaction(
        self,
        lease: ExecutionLease,
        *,
        message: MessageReference,
        failure: JobExecutionFailure,
        now: datetime | None = None,
    ) -> None:
        """Persist failure, inbox identity, and retry dispatch in the caller's transaction."""
        occurred_at = now or self._clock()
        model = self._lock_job(lease.job_id)
        self._require_current_lease(model, lease, occurred_at)
        if model.cancel_requested_at is not None:
            model.status = JobStatus.CANCELLED.value
            model.lease_owner = None
            model.lease_expires_at = None
            model.completed_at = occurred_at
            model.last_error_code = None
            model.last_error_category = None
            model.last_error_at = None
            model.next_action = None
            model.manual_retry_allowed = False
            model.defer_reason = None
            model.next_run_at = None
            model.updated_at = occurred_at
            attempt_outcome = "cancelled"
            should_retry = False
            next_retry_count = model.retry_count
        else:
            next_retry_count = model.retry_count + 1
            automatic = failure.category in {
                JobFailureCategory.TRANSIENT,
                JobFailureCategory.RATE_LIMITED,
            }
            should_retry = (
                automatic
                and failure.retry_at is not None
                and failure.max_attempts is not None
                and failure.retry_at > occurred_at
                and next_retry_count < failure.max_attempts
            )

            model.lease_owner = None
            model.lease_expires_at = None
            model.last_error_code = failure.error_code
            model.last_error_category = failure.category.value
            model.last_error_at = failure.occurred_at
            model.next_action = failure.next_action
            model.manual_retry_allowed = failure.manual_retry_allowed
            model.updated_at = occurred_at
            if should_retry:
                model.retry_count = next_retry_count
            model.status = JobStatus.QUEUED.value if should_retry else JobStatus.FAILED.value
            model.completed_at = None if should_retry else occurred_at
            model.defer_reason = failure.category.value if should_retry else None
            model.next_run_at = failure.retry_at if should_retry else None
            attempt_outcome = "delayed" if should_retry else "failed"

        self._session.execute(
            update(JobAttempt)
            .where(
                JobAttempt.job_id == model.id,
                JobAttempt.lease_epoch == model.lease_epoch,
                JobAttempt.finished_at.is_(None),
            )
            .values(finished_at=occurred_at, outcome=attempt_outcome)
        )
        self._session.add(
            ProcessedMessage(
                id=message.message_id,
                job_id=model.id,
                topic=message.topic,
                partition=message.partition,
                message_offset=message.offset,
                processed_at=occurred_at,
            )
        )
        if should_retry:
            if failure.retry_at is None:
                raise RuntimeError("scheduled retry is missing retry_at")
            dispatch_sequence = (
                self._session.scalar(
                    select(func.max(OutboxMessage.dispatch_sequence)).where(
                        OutboxMessage.aggregate_id == model.id
                    )
                )
                or 0
            ) + 1
            self._session.add(
                OutboxMessage(
                    id=uuid4(),
                    aggregate_id=model.id,
                    topic="hotkey.jobs.accepted.v2",
                    message_key=model.id,
                    event_type="job.retry_scheduled.v1",
                    dispatch_sequence=dispatch_sequence,
                    payload={
                        "job_id": str(model.id),
                        "owner_id": str(model.owner_id),
                        "operation_id": str(model.operation_id),
                        "kind": model.kind,
                        "configuration_ref": model.configuration_ref,
                        "configuration_version": model.configuration_version,
                        "source_key": model.source_key,
                        "source_capability": model.source_capability,
                        "dispatch_sequence": dispatch_sequence,
                        "retry_count": next_retry_count,
                        "retry_at": _utc_text(failure.retry_at),
                        "last_error_code": failure.error_code,
                    },
                    available_at=failure.retry_at,
                    created_at=occurred_at,
                    published_at=None,
                )
            )

    def is_processed(self, message_id: UUID) -> bool:
        processed = self._session.get(ProcessedMessage, message_id) is not None
        self._session.rollback()
        return processed

    def current_lease_expiration(self, job_id: UUID) -> datetime | None:
        model = self._session.get(Job, job_id)
        expires_at = model.lease_expires_at if model is not None else None
        self._session.rollback()
        return expires_at

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
