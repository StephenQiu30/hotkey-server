from __future__ import annotations

from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

from sqlalchemy import func, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session

from connections.schemas import SourceExecutionPolicy
from jobs.models import (
    CollectionDueWindow,
    CoverageWindow,
    Job,
    ResourceBudgetReservation,
    ResourceUsageAttempt,
)
from jobs.schemas import (
    AnalysisJobFactView,
    CollectionDueWindowInput,
    CollectionDueWindowView,
    CollectionExecutionFactView,
    CoverageWindowStatus,
    DueAdmissionState,
    DueSkipReason,
    JobStatus,
)
from sources.contracts import SourceCapability


class CollectionDueConflictError(ValueError):
    """The same scheduled instant has incompatible immutable facts or admission."""


def _as_utc(value: datetime) -> datetime:
    if value.utcoffset() is None:
        raise ValueError("due window timestamps must be timezone-aware")
    return value.astimezone(UTC)


def _automatic_skip(command: CollectionDueWindowInput) -> DueSkipReason | None:
    policy = command.policy_snapshot
    if policy is None:
        return None
    if not policy.enabled:
        return DueSkipReason.DISABLED
    if policy.quiet_at(command.due_at):
        return DueSkipReason.QUIET
    return None


class CollectionDueWindowService:
    """Own due facts; the scheduler and JobService share the caller's transaction."""

    def __init__(self, session: Session, *, clock: Callable[[], datetime] | None = None) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def record_due_in_transaction(
        self, command: CollectionDueWindowInput
    ) -> CollectionDueWindowView:
        self._require_transaction()
        now = _as_utc(self._clock())
        skip = _automatic_skip(command)
        self._session.execute(
            insert(CollectionDueWindow)
            .values(
                id=uuid4(),
                owner_id=command.owner_id,
                schedule_key=command.schedule_key,
                topic_id=command.topic_id,
                source_key=command.source_key,
                capability=command.capability.value,
                due_at=command.due_at,
                window_start=command.window_start,
                window_end=command.window_end,
                connection_version=command.connection_version,
                policy_snapshot=(
                    command.policy_snapshot.model_dump(mode="json")
                    if command.policy_snapshot is not None
                    else None
                ),
                admission_state=(DueAdmissionState.SKIPPED if skip else DueAdmissionState.PENDING),
                reason=skip.value if skip else None,
                recorded_at=now,
            )
            .on_conflict_do_nothing(constraint="collection_due_windows_owner_schedule_due_key")
        )
        model = self._lock(command.owner_id, command.schedule_key, command.due_at)
        if (
            model.topic_id != command.topic_id
            or model.source_key != command.source_key
            or model.capability != command.capability.value
            or _as_utc(model.window_start) != command.window_start
            or _as_utc(model.window_end) != command.window_end
            or model.connection_version != command.connection_version
            or model.policy_snapshot
            != (
                command.policy_snapshot.model_dump(mode="json")
                if command.policy_snapshot is not None
                else None
            )
        ):
            raise CollectionDueConflictError("scheduled instant conflicts with its frozen facts")
        return self._view(model)

    def record_elapsed_in_transaction(
        self,
        *,
        first_due: CollectionDueWindowInput,
        interval_seconds: int,
        through_at: datetime,
        snapshot_at: Callable[[datetime], tuple[int | None, SourceExecutionPolicy | None]],
    ) -> tuple[CollectionDueWindowView, ...]:
        """Replay deterministic due instants after a stopped scan, without inventing jobs."""
        self._require_transaction()
        through = _as_utc(through_at)
        if not 1 <= interval_seconds <= 86_400 or first_due.due_at > through:
            raise ValueError("recovery needs a bounded interval and a prior due point")
        cadence = timedelta(seconds=interval_seconds)
        count = (through - first_due.due_at) // cadence + 1
        latest = self._session.scalar(
            select(func.max(CollectionDueWindow.due_at)).where(
                CollectionDueWindow.owner_id == first_due.owner_id,
                CollectionDueWindow.schedule_key == first_due.schedule_key,
            )
        )
        start_index = 0
        if latest is not None:
            elapsed = _as_utc(latest) - first_due.due_at
            if elapsed < timedelta(0) or elapsed % cadence:
                raise CollectionDueConflictError(
                    "last persisted due is outside the recovery cadence"
                )
            start_index = elapsed // cadence
        if count - start_index > 1000:
            raise ValueError("recover at most 1000 due points per transaction")
        result: list[CollectionDueWindowView] = []
        for index in range(start_index, count):
            offset = timedelta(seconds=index * interval_seconds)
            due_at = first_due.due_at + offset
            connection_version, policy_snapshot = snapshot_at(due_at)
            command = first_due.model_copy(
                update={
                    "due_at": due_at,
                    "window_start": first_due.window_start + offset,
                    "window_end": first_due.window_end + offset,
                    "connection_version": connection_version,
                    "policy_snapshot": policy_snapshot,
                }
            )
            recorded = self.record_due_in_transaction(command)
            if recorded.admission_state is DueAdmissionState.PENDING and index < count - 1:
                recorded = self.mark_missed_in_transaction(
                    owner_id=command.owner_id,
                    schedule_key=command.schedule_key,
                    due_at=command.due_at,
                )
            result.append(recorded)
        return tuple(result)

    def mark_accepted_in_transaction(
        self,
        *,
        owner_id: UUID,
        schedule_key: UUID,
        due_at: datetime,
        operation_id: UUID,
        job_id: UUID,
    ) -> CollectionDueWindowView:
        self._require_transaction()
        model = self._lock(owner_id, schedule_key, due_at)
        job = self._session.get(Job, job_id)
        if (
            job is None
            or job.owner_id != owner_id
            or job.operation_id != operation_id
            or job.source_key != model.source_key
            or job.source_capability != model.capability
            or model.connection_version is None
            or model.policy_snapshot is None
            or job.scope.get("connection_version") != model.connection_version
            or (model.topic_id is not None and job.configuration_ref != f"topic:{model.topic_id}")
        ):
            raise CollectionDueConflictError("accepted due requires an existing owner-scoped job")
        if model.admission_state == DueAdmissionState.ACCEPTED:
            if model.job_id != job_id or model.operation_id != operation_id:
                raise CollectionDueConflictError("due was accepted for another job")
            return self._view(model)
        if model.admission_state != DueAdmissionState.PENDING:
            raise CollectionDueConflictError("only a pending due may be accepted")
        model.admission_state = DueAdmissionState.ACCEPTED.value
        model.operation_id = operation_id
        model.job_id = job_id
        return self._view(model)

    def mark_skipped_in_transaction(
        self,
        *,
        owner_id: UUID,
        schedule_key: UUID,
        due_at: datetime,
        reason: DueSkipReason,
    ) -> CollectionDueWindowView:
        self._require_transaction()
        model = self._lock(owner_id, schedule_key, due_at)
        if model.admission_state == DueAdmissionState.SKIPPED and model.reason == reason.value:
            return self._view(model)
        if model.admission_state != DueAdmissionState.PENDING:
            raise CollectionDueConflictError("only a pending due may be skipped")
        model.admission_state = DueAdmissionState.SKIPPED.value
        model.reason = reason.value
        return self._view(model)

    def mark_missed_in_transaction(
        self, *, owner_id: UUID, schedule_key: UUID, due_at: datetime
    ) -> CollectionDueWindowView:
        self._require_transaction()
        model = self._lock(owner_id, schedule_key, due_at)
        if model.admission_state == DueAdmissionState.MISSED:
            return self._view(model)
        if model.admission_state != DueAdmissionState.PENDING:
            raise CollectionDueConflictError("only a pending due may be marked missed")
        model.admission_state = DueAdmissionState.MISSED.value
        model.reason = "scheduler_interrupted"
        return self._view(model)

    def list_due(
        self, *, owner_id: UUID, start: datetime, end: datetime
    ) -> tuple[CollectionDueWindowView, ...]:
        start, end = _as_utc(start), _as_utc(end)
        if start >= end:
            raise ValueError("due query must be a forward half-open range")
        rows = self._session.scalars(
            select(CollectionDueWindow)
            .where(
                CollectionDueWindow.owner_id == owner_id,
                CollectionDueWindow.due_at >= start,
                CollectionDueWindow.due_at < end,
            )
            .order_by(CollectionDueWindow.due_at, CollectionDueWindow.schedule_key)
        ).all()
        return tuple(self._view(row) for row in rows)

    def list_execution_facts(
        self, *, owner_id: UUID, start: datetime, end: datetime
    ) -> tuple[CollectionExecutionFactView, ...]:
        """Project job, attempt, budget and coverage facts without content ORM access."""
        result: list[CollectionExecutionFactView] = []
        for due in self.list_due(owner_id=owner_id, start=start, end=end):
            if due.job_id is None:
                result.append(
                    CollectionExecutionFactView(
                        due=due,
                        job_status=None,
                        requests_sent=None,
                        request_attempt_count=None,
                        charged_request_count=None,
                        request_budget_reconciled=None,
                        page_count=None,
                        observed_count=None,
                        coverage_status=None,
                        stop_reason=due.reason,
                        has_gap=True,
                    )
                )
                continue
            job = self._session.get(Job, due.job_id)
            if job is None or job.owner_id != owner_id or job.operation_id != due.operation_id:
                raise RuntimeError("accepted due has no owner-scoped job")
            usage_attempts = self._session.scalars(
                select(ResourceUsageAttempt).where(
                    ResourceUsageAttempt.owner_id == owner_id,
                    ResourceUsageAttempt.operation_id == job.operation_id,
                    ResourceUsageAttempt.usage_kind == "network_request",
                )
            ).all()
            reservations = self._session.scalars(
                select(ResourceBudgetReservation).where(
                    ResourceBudgetReservation.owner_id == owner_id,
                    ResourceBudgetReservation.operation_id == job.operation_id,
                    ResourceBudgetReservation.metric == "network_request",
                )
            ).all()
            budget_rows_by_attempt: dict[UUID, list[ResourceBudgetReservation]] = {}
            for row in reservations:
                budget_rows_by_attempt.setdefault(row.reservation_id, []).append(row)
            attempted_ids = {row.attempt_id for row in usage_attempts}
            reconciled = (
                job.requests_sent == len(attempted_ids)
                and attempted_ids == set(budget_rows_by_attempt)
                and all(
                    row.status == "settled" and row.actual_units == 1
                    for rows in budget_rows_by_attempt.values()
                    for row in rows
                )
            )
            charged_count = len(attempted_ids) if reconciled else None
            coverage = self._session.scalars(
                select(CoverageWindow).where(
                    CoverageWindow.owner_id == owner_id,
                    CoverageWindow.last_job_id == job.id,
                )
            ).all()
            page_count = sum(row.page_count for row in coverage)
            observed = job.checkpoint.get("collection.observed_count")
            if observed is not None and (type(observed) is not int or observed < 0):
                raise RuntimeError("collection observed count checkpoint is invalid")
            if observed is None and page_count == 0 and job.requests_sent == 0:
                observed = 0
            if observed is None and job.last_error_code == "hotlist_source_empty":
                observed = 0
            if job.kind == "source.hotlist" and job.checkpoint.get("snapshot_id") is not None:
                page_count = 1
            confirmed = bool(coverage) and all(row.status == "confirmed" for row in coverage)
            status = (
                CoverageWindowStatus.CONFIRMED
                if confirmed
                else CoverageWindowStatus.PARTIAL
                if any(row.status == "partial" for row in coverage)
                else CoverageWindowStatus.RUNNING
                if any(row.status == "running" for row in coverage)
                else None
            )
            result.append(
                CollectionExecutionFactView(
                    due=due,
                    job_status=JobStatus(job.status),
                    requests_sent=job.requests_sent,
                    request_attempt_count=len(attempted_ids),
                    charged_request_count=charged_count,
                    request_budget_reconciled=reconciled,
                    page_count=page_count,
                    observed_count=observed,
                    coverage_status=status,
                    stop_reason=(
                        next((row.stop_reason for row in coverage if row.stop_reason), None)
                        or job.last_error_code
                    ),
                    has_gap=not confirmed or not reconciled,
                )
            )
        return tuple(result)

    def list_analysis_jobs_in_transaction(
        self, *, owner_id: UUID
    ) -> tuple[AnalysisJobFactView, ...]:
        if not self._session.in_transaction():
            raise RuntimeError("analysis job reads require the caller's transaction")
        rows = self._session.scalars(
            select(Job).where(Job.owner_id == owner_id, Job.kind == "analysis.annotate")
        ).all()
        return tuple(
            AnalysisJobFactView(
                id=row.id,
                status=JobStatus(row.status),
                scope=row.scope,
                last_error_code=row.last_error_code,
            )
            for row in rows
        )

    def _lock(self, owner_id: UUID, schedule_key: UUID, due_at: datetime) -> CollectionDueWindow:
        model = self._session.scalar(
            select(CollectionDueWindow)
            .where(
                CollectionDueWindow.owner_id == owner_id,
                CollectionDueWindow.schedule_key == schedule_key,
                CollectionDueWindow.due_at == _as_utc(due_at),
            )
            .with_for_update()
        )
        if model is None:
            raise CollectionDueConflictError("due window is not recorded")
        return model

    def _require_transaction(self) -> None:
        if not self._session.in_transaction():
            raise RuntimeError("due mutations require the caller's transaction")

    @staticmethod
    def _view(model: CollectionDueWindow) -> CollectionDueWindowView:
        return CollectionDueWindowView(
            id=model.id,
            owner_id=model.owner_id,
            schedule_key=model.schedule_key,
            topic_id=model.topic_id,
            source_key=model.source_key,
            capability=SourceCapability(model.capability),
            due_at=_as_utc(model.due_at),
            window_start=_as_utc(model.window_start),
            window_end=_as_utc(model.window_end),
            connection_version=model.connection_version,
            policy_snapshot=(
                SourceExecutionPolicy.model_validate(model.policy_snapshot)
                if model.policy_snapshot is not None
                else None
            ),
            admission_state=DueAdmissionState(model.admission_state),
            reason=model.reason,
            operation_id=model.operation_id,
            job_id=model.job_id,
            recorded_at=_as_utc(model.recorded_at),
        )
