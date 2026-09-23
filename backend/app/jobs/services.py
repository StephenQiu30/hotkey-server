from __future__ import annotations

import hashlib
import json
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

from sqlalchemy import func, or_, select, update
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session

from connections.services import require_source_connection_enabled
from core.errors import ApplicationError
from jobs.cursor import (
    CursorPageProgress,
    CursorPageRequest,
    advance_cursor_page,
    plan_cursor_request,
)
from jobs.execution import (
    ExecutionLease,
    JobExecutionService,
    JobProgress,
    ScheduleWindow,
    scheduled_operation_id,
)
from jobs.models import (
    CoverageWindow,
    Job,
    JobAttempt,
    JobStageAttempt,
    OutboxMessage,
    ResourceBudgetPolicy,
    ResourceBudgetReservation,
    ResourceBudgetWindow,
    ResourceComponentPolicy,
    ResourceUsageAttempt,
)
from jobs.schemas import (
    BudgetDecisionStatus,
    BudgetMetric,
    BudgetPolicyInput,
    BudgetPolicyView,
    BudgetReservationDecision,
    BudgetReservationInput,
    BudgetReservationStatus,
    BudgetResumeCondition,
    BudgetScopeKind,
    BudgetSettlementView,
    CollectionScanKind,
    ComponentPolicyInput,
    ComponentPolicyView,
    CostClass,
    CoverageTerminalEvidence,
    CoverageWindowInput,
    CoverageWindowView,
    FreshnessTimelineInput,
    FreshnessTimelineView,
    JobAcceptanceInput,
    JobCancellationView,
    JobControlStatus,
    JobFailureCategory,
    JobFailureView,
    JobObservationContext,
    JobProgressView,
    JobStage,
    JobStageOutcome,
    JobStatus,
    JobStatusView,
    JobView,
    OperationalSnapshot,
    OperationalSummary,
    OperationalTaskRecord,
    OperationalTaskStatus,
    OperationAttemptCount,
    SourceTimeStatus,
    StageAttemptInput,
    StageAttemptView,
    UsageAttemptInput,
    UsageAttemptView,
    UsageOutcome,
    UsageSummaryView,
)
from sources.contracts import SourceCapability, SourcePageState, SourceStopReason

JOB_ACCEPTED_EVENT_TYPE = "job.accepted.v2"
JOB_ACCEPTED_TOPIC = "hotkey.jobs.accepted.v2"
JOB_RETRY_EVENT_TYPE = "job.retry_scheduled.v1"
JOB_EVENT_SCHEMA_VERSIONS = {
    JOB_ACCEPTED_EVENT_TYPE: 2,
    JOB_RETRY_EVENT_TYPE: 1,
}

type PublishOutbox = Callable[["OutboxEnvelope"], None]
type OutboxValue = str | int | bool | None


class CoverageWindowConflictError(RuntimeError):
    """The page cannot advance the selected durable source range."""


def _scan_kind(scope: Mapping[str, OutboxValue]) -> CollectionScanKind | None:
    value = scope.get("scan_kind")
    if not isinstance(value, str):
        return None
    try:
        return CollectionScanKind(value)
    except ValueError:
        return None


class CoverageWindowService:
    def __init__(
        self,
        session: Session,
        *,
        execution: JobExecutionService,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._session = session
        self._execution = execution
        self._clock = clock or (lambda: datetime.now(UTC))

    def begin_in_transaction(
        self,
        *,
        lease: ExecutionLease,
        window: CoverageWindowInput,
    ) -> CoverageWindowView:
        job = self._require_job(lease=lease, window=window)
        now = self._clock()
        self._session.execute(
            insert(CoverageWindow)
            .values(
                id=uuid4(),
                owner_id=window.owner_id,
                source_key=window.source_key,
                capability=window.capability.value,
                target_hash=window.target_hash,
                sort_key=window.sort_key.value,
                rule_version=window.rule_version,
                starts_at=window.starts_at,
                ends_at=window.ends_at,
                status="pending",
                checkpoint_sequence=0,
                page_count=0,
                created_at=now,
                updated_at=now,
            )
            .on_conflict_do_nothing(
                constraint="coverage_windows_scope_range_key",
            )
        )
        model = self._lock_window(window)
        if model.status == "confirmed":
            return self._view(model)
        if model.status == "running":
            if model.last_job_id == job.id:
                return self._view(model)
            if model.last_job_id is not None:
                raise CoverageWindowConflictError("coverage window is owned by another job")
        model.status = "running"
        model.stop_reason = None
        model.last_job_id = job.id
        model.checkpoint_sequence = job.checkpoint_sequence
        model.updated_at = now
        return self._view(model)

    def record_page_in_transaction(
        self,
        *,
        lease: ExecutionLease,
        window: CoverageWindowInput,
        page_state: SourcePageState,
        stop_reason: SourceStopReason | None = None,
        evidence: CoverageTerminalEvidence | None = None,
    ) -> CoverageWindowView:
        job = self._require_job(lease=lease, window=window)
        model = self._lock_window(window)
        if job.checkpoint_sequence != lease.checkpoint_sequence:
            raise CoverageWindowConflictError("page checkpoint does not match the current job")
        if job.checkpoint_sequence == 0:
            raise CoverageWindowConflictError("a page requires a persisted checkpoint")
        if model.last_job_id == job.id and job.checkpoint_sequence == model.checkpoint_sequence:
            return self._view(model)
        if model.status != "running" or model.last_job_id != job.id:
            raise CoverageWindowConflictError("coverage window has not been opened by this job")
        if job.checkpoint_sequence < model.checkpoint_sequence:
            raise CoverageWindowConflictError("page checkpoint moved backwards")

        if page_state in {SourcePageState.PARTIAL, SourcePageState.STOPPED}:
            if stop_reason is None or evidence is not None:
                raise ValueError("stopped pages require a reason and no terminal evidence")
            model.status = "partial"
            model.stop_reason = stop_reason.value
        elif page_state in {SourcePageState.COMPLETE, SourcePageState.EMPTY}:
            if stop_reason is not None:
                raise ValueError("terminal pages cannot carry a stop reason")
            if self._proves_window(window=window, evidence=evidence):
                model.status = "confirmed"
                model.stop_reason = None
            else:
                model.status = "partial"
                model.stop_reason = "unverified_terminal"
        elif page_state is SourcePageState.MORE:
            if stop_reason is not None or evidence is not None:
                raise ValueError("continuing pages cannot carry terminal evidence")
        else:
            raise ValueError("unsupported source page state")

        model.checkpoint_sequence = job.checkpoint_sequence
        model.page_count += 1
        model.updated_at = self._clock()
        return self._view(model)

    def record_cursor_page_in_transaction(
        self,
        *,
        lease: ExecutionLease,
        window: CoverageWindowInput,
        request: CursorPageRequest,
        page_state: SourcePageState,
        next_token: str | None = None,
        stop_reason: SourceStopReason | None = None,
        evidence: CoverageTerminalEvidence | None = None,
        job_progress: JobProgress | None = None,
    ) -> tuple[ExecutionLease, CoverageWindowView, CursorPageProgress]:
        """Commit a bounded cursor page and range progress in the caller's transaction."""
        expected = plan_cursor_request(
            window=window,
            checkpoint=lease.checkpoint,
            live_token=request.token,
            max_pages=request.max_pages,
            max_rescans=request.max_rescans,
        )
        if request != expected:
            raise CoverageWindowConflictError("cursor request does not match the job checkpoint")
        progress = advance_cursor_page(
            request,
            state=page_state,
            next_token=next_token,
            stop_reason=stop_reason,
        )
        opened = self.begin_in_transaction(lease=lease, window=window)
        if opened.status == "confirmed":
            raise CoverageWindowConflictError("confirmed window cannot accept another page")
        updated = self._execution.save_checkpoint_in_transaction(
            lease,
            sequence=lease.checkpoint_sequence + 1,
            checkpoint=progress.checkpoint,
            progress=job_progress,
        )
        view = self.record_page_in_transaction(
            lease=updated,
            window=window,
            page_state=progress.state,
            stop_reason=progress.stop_reason,
            evidence=evidence,
        )
        return updated, view, progress

    def confirmed_through(self, *, window: CoverageWindowInput, from_at: datetime) -> datetime:
        if from_at.utcoffset() != timedelta(0):
            raise ValueError("watermark origin must be UTC")
        cursor = from_at
        rows = self._session.scalars(
            select(CoverageWindow)
            .where(
                CoverageWindow.owner_id == window.owner_id,
                CoverageWindow.source_key == window.source_key,
                CoverageWindow.capability == window.capability.value,
                CoverageWindow.target_hash == window.target_hash,
                CoverageWindow.sort_key == window.sort_key.value,
                CoverageWindow.rule_version == window.rule_version,
                CoverageWindow.ends_at > from_at,
            )
            .order_by(CoverageWindow.starts_at, CoverageWindow.ends_at)
        )
        for row in rows:
            if row.starts_at > cursor or row.status != "confirmed":
                break
            cursor = max(cursor, row.ends_at)
        return cursor

    def _require_job(self, *, lease: ExecutionLease, window: CoverageWindowInput) -> Job:
        self._execution.require_current_lease_in_transaction(lease)
        job = self._session.get(Job, lease.job_id)
        if job is None or job.owner_id != window.owner_id:
            raise CoverageWindowConflictError("coverage window owner does not match the job")
        if job.source_key != window.source_key or job.source_capability != window.capability.value:
            raise CoverageWindowConflictError("coverage window source does not match the job")
        if (
            job.scope.get("target_hash") != window.target_hash.hex()
            or job.scope.get("sort_key") != window.sort_key.value
            or job.scope.get("rule_version") != window.rule_version
        ):
            raise CoverageWindowConflictError("coverage window scope does not match the job")
        if _scan_kind(job.scope) is None:
            raise CoverageWindowConflictError("coverage window requires an explicit scan kind")
        return job

    def _lock_window(self, window: CoverageWindowInput) -> CoverageWindow:
        model = self._session.scalar(
            select(CoverageWindow)
            .where(
                CoverageWindow.owner_id == window.owner_id,
                CoverageWindow.source_key == window.source_key,
                CoverageWindow.capability == window.capability.value,
                CoverageWindow.target_hash == window.target_hash,
                CoverageWindow.sort_key == window.sort_key.value,
                CoverageWindow.rule_version == window.rule_version,
                CoverageWindow.starts_at == window.starts_at,
                CoverageWindow.ends_at == window.ends_at,
            )
            .with_for_update()
        )
        if model is None:
            raise CoverageWindowConflictError("coverage window does not exist")
        return model

    @staticmethod
    def _proves_window(
        *, window: CoverageWindowInput, evidence: CoverageTerminalEvidence | None
    ) -> bool:
        return bool(
            evidence is not None
            and evidence.starts_at == window.starts_at
            and evidence.ends_at == window.ends_at
            and evidence.sort_key == window.sort_key
            and evidence.query_bounded
            and evidence.sort_applied
            and evidence.terminal_verified
        )

    @staticmethod
    def _view(model: CoverageWindow) -> CoverageWindowView:
        return CoverageWindowView(
            id=model.id,
            status=model.status,
            stop_reason=model.stop_reason,
            page_count=model.page_count,
        )


class ResourceBudgetError(RuntimeError):
    """Base class for resource policy and metering conflicts."""


class ComponentPolicyUnavailableError(ResourceBudgetError):
    """The requested component is absent or not eligible for the core path."""


class UsageConflictError(ResourceBudgetError):
    """An attempt identifier was replayed with conflicting data or outcome."""


class BudgetPolicyConflictError(ResourceBudgetError):
    """A budget key was reused with incompatible structural fields."""


class BudgetPolicyUnavailableError(ResourceBudgetError):
    """No complete active budget policy applies to the requested work."""


class BudgetReservationConflictError(ResourceBudgetError):
    """A reservation identifier or settlement was replayed inconsistently."""


class StageAttemptConflictError(RuntimeError):
    """A stage attempt identifier or sequence was replayed with different facts."""


class StageAttemptUnavailableError(RuntimeError):
    """A task or stage attempt is outside the current owner scope."""


@dataclass(frozen=True, slots=True)
class OutboxEnvelope:
    message_id: UUID
    topic: str
    message_key: UUID
    event_type: str
    schema_version: int
    payload: dict[str, OutboxValue]

    def message_body(self) -> dict[str, OutboxValue]:
        return {
            **self.payload,
            "schema_version": self.schema_version,
            "message_id": str(self.message_id),
            "event_type": self.event_type,
        }


@dataclass(frozen=True, slots=True)
class ContentJobContext:
    job_id: UUID
    configuration_ref: str
    configuration_version: int
    source_key: str
    source_capability: SourceCapability
    scan_kind: CollectionScanKind | None


@dataclass(frozen=True, slots=True)
class JobExecutionConfiguration:
    job_id: UUID
    owner_id: UUID
    operation_id: UUID
    kind: str
    observation: JobObservationContext
    scope: dict[str, OutboxValue]


def load_job_execution_configuration(
    session: Session,
    *,
    job_id: UUID,
) -> JobExecutionConfiguration | None:
    """Load server-owned execution input without controlling the transaction."""
    job = session.get(Job, job_id)
    return _job_execution_configuration(job) if job is not None else None


def load_job_execution_configuration_by_operation(
    session: Session,
    *,
    owner_id: UUID,
    kind: str,
    operation_id: UUID,
) -> JobExecutionConfiguration | None:
    """Find an accepted job replay without re-evaluating mutable source state."""
    job = session.scalar(
        select(Job).where(
            Job.owner_id == owner_id,
            Job.kind == kind,
            Job.operation_id == operation_id,
        )
    )
    return _job_execution_configuration(job) if job is not None else None


def _job_execution_configuration(job: Job) -> JobExecutionConfiguration:
    return JobExecutionConfiguration(
        job_id=job.id,
        owner_id=job.owner_id,
        operation_id=job.operation_id,
        kind=job.kind,
        observation=JobObservationContext(
            configuration_ref=job.configuration_ref,
            configuration_version=job.configuration_version,
            source_key=job.source_key,
            source_capability=(
                SourceCapability(job.source_capability)
                if job.source_capability is not None
                else None
            ),
        ),
        scope=dict(job.scope),
    )


def load_content_job_context(
    session: Session,
    *,
    owner_id: UUID,
    job_id: UUID,
) -> ContentJobContext | None:
    """Read the source job context without owning the caller's transaction."""
    job = session.scalar(select(Job).where(Job.owner_id == owner_id, Job.id == job_id))
    if job is None or job.source_key is None or job.source_capability is None:
        return None
    return ContentJobContext(
        job_id=job.id,
        configuration_ref=job.configuration_ref,
        configuration_version=job.configuration_version,
        source_key=job.source_key,
        source_capability=SourceCapability(job.source_capability),
        scan_kind=_scan_kind(job.scope),
    )


def load_content_job_contexts(
    session: Session,
    *,
    owner_id: UUID,
    job_ids: set[UUID],
) -> dict[UUID, ContentJobContext]:
    """Batch-read source job context for content projections."""
    if not job_ids:
        return {}
    jobs = session.scalars(select(Job).where(Job.owner_id == owner_id, Job.id.in_(job_ids))).all()
    return {
        job.id: ContentJobContext(
            job_id=job.id,
            configuration_ref=job.configuration_ref,
            configuration_version=job.configuration_version,
            source_key=job.source_key,
            source_capability=SourceCapability(job.source_capability),
            scan_kind=_scan_kind(job.scope),
        )
        for job in jobs
        if job.source_key is not None and job.source_capability is not None
    }


def fingerprint_request(command: JobAcceptanceInput) -> bytes:
    canonical = json.dumps(
        {
            "kind": command.kind,
            "observation": command.observation.model_dump(mode="json"),
            "scope": command.scope,
        },
        ensure_ascii=False,
        allow_nan=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return hashlib.sha256(canonical.encode()).digest()


def _duration_us(start: datetime | None, end: datetime | None) -> int | None:
    if start is None or end is None:
        return None
    duration = end - start
    return (duration.days * 86_400 + duration.seconds) * 1_000_000 + duration.microseconds


def _as_utc(value: datetime | None) -> datetime | None:
    return value.astimezone(UTC) if value is not None else None


def measure_freshness_timeline(command: FreshnessTimelineInput) -> FreshnessTimelineView:
    published_at = command.source_published_at
    observed_at = command.source_observed_at
    if published_at is None:
        source_status = SourceTimeStatus.UNKNOWN
    elif published_at.utcoffset() is None:
        source_status = SourceTimeStatus.MISSING_TIMEZONE
    elif observed_at is not None and published_at > observed_at:
        source_status = SourceTimeStatus.FUTURE_SKEW
    else:
        source_status = SourceTimeStatus.VALID

    publication_delay = None
    if source_status is SourceTimeStatus.VALID:
        publication_delay = _duration_us(published_at, observed_at)

    return FreshnessTimelineView(
        **command.model_dump(),
        source_time_status=source_status,
        schedule_wait_us=_duration_us(command.scheduled_for_at, command.started_at),
        queue_wait_us=_duration_us(command.accepted_at, command.started_at),
        internal_prepare_us=_duration_us(command.started_at, command.request_started_at),
        source_wait_us=_duration_us(command.request_started_at, observed_at),
        processing_us=_duration_us(observed_at, command.persisted_at),
        visibility_us=_duration_us(command.persisted_at, command.queryable_at),
        end_to_end_us=_duration_us(command.scheduled_for_at, command.queryable_at),
        publication_to_observation_us=publication_delay,
    )


class ResourceBudgetService:
    _CORE_COST_CLASSES = frozenset({CostClass.LOCAL.value, CostClass.ZERO_PRICE.value})

    def __init__(
        self,
        session: Session,
        *,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def save_component_policy(
        self,
        *,
        owner_id: UUID,
        command: ComponentPolicyInput,
    ) -> ComponentPolicyView:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            statement = insert(ResourceComponentPolicy).values(
                id=uuid4(),
                owner_id=owner_id,
                component_key=command.component_key,
                component_version=command.component_version,
                cost_class=command.cost_class.value,
                enabled_for_core=command.enabled_for_core,
                terms_reference=command.terms_reference,
                reviewed_at=command.reviewed_at,
                policy_version=1,
                created_at=now,
                updated_at=now,
            )
            policy_id = self._session.scalar(
                statement.on_conflict_do_update(
                    constraint="resource_component_policies_owner_component_key",
                    set_={
                        "component_version": statement.excluded.component_version,
                        "cost_class": statement.excluded.cost_class,
                        "enabled_for_core": statement.excluded.enabled_for_core,
                        "terms_reference": statement.excluded.terms_reference,
                        "reviewed_at": statement.excluded.reviewed_at,
                        "policy_version": ResourceComponentPolicy.policy_version + 1,
                        "updated_at": now,
                    },
                    where=or_(
                        ResourceComponentPolicy.component_version
                        != statement.excluded.component_version,
                        ResourceComponentPolicy.cost_class != statement.excluded.cost_class,
                        ResourceComponentPolicy.enabled_for_core
                        != statement.excluded.enabled_for_core,
                        ResourceComponentPolicy.terms_reference
                        != statement.excluded.terms_reference,
                        ResourceComponentPolicy.reviewed_at != statement.excluded.reviewed_at,
                    ),
                ).returning(ResourceComponentPolicy.id)
            )
            if policy_id is None:
                model = self._session.scalar(
                    select(ResourceComponentPolicy).where(
                        ResourceComponentPolicy.owner_id == owner_id,
                        ResourceComponentPolicy.component_key == command.component_key,
                    )
                )
            else:
                model = self._session.get(ResourceComponentPolicy, policy_id)
            if model is None:
                raise RuntimeError("saved component policy is not visible")
            view = self._component_policy_view(model)
        return view

    def begin_attempt(
        self,
        *,
        owner_id: UUID,
        command: UsageAttemptInput,
    ) -> UsageAttemptView:
        self._session.rollback()
        with self._session.begin():
            return self.begin_attempt_in_transaction(owner_id=owner_id, command=command)

    def begin_attempt_in_transaction(
        self,
        *,
        owner_id: UUID,
        command: UsageAttemptInput,
    ) -> UsageAttemptView:
        """Persist a usage attempt inside an existing outer transaction."""
        existing = self._session.scalar(
            select(ResourceUsageAttempt).where(
                ResourceUsageAttempt.owner_id == owner_id,
                ResourceUsageAttempt.attempt_id == command.attempt_id,
            )
        )
        if existing is not None:
            self._require_same_attempt(existing, command)
            return self._attempt_view(existing)

        policy = self._session.scalar(
            select(ResourceComponentPolicy)
            .where(
                ResourceComponentPolicy.owner_id == owner_id,
                ResourceComponentPolicy.component_key == command.component_key,
            )
            .with_for_update()
        )
        if (
            policy is None
            or not policy.enabled_for_core
            or policy.cost_class not in self._CORE_COST_CLASSES
        ):
            raise ComponentPolicyUnavailableError(
                "component is not enabled for zero-cost core execution"
            )

        usage_id = uuid4()
        inserted_id = self._session.scalar(
            insert(ResourceUsageAttempt)
            .values(
                id=usage_id,
                owner_id=owner_id,
                attempt_id=command.attempt_id,
                operation_id=command.operation_id,
                component_policy_id=policy.id,
                component_version=policy.component_version,
                usage_kind=command.usage_kind.value,
                stage=command.stage,
                outcome=UsageOutcome.STARTED.value,
                started_at=command.started_at,
                finished_at=None,
            )
            .on_conflict_do_nothing(constraint="resource_usage_attempts_owner_attempt_key")
            .returning(ResourceUsageAttempt.id)
        )
        if inserted_id is not None:
            model = self._session.get(ResourceUsageAttempt, inserted_id)
        else:
            model = self._session.scalar(
                select(ResourceUsageAttempt).where(
                    ResourceUsageAttempt.owner_id == owner_id,
                    ResourceUsageAttempt.attempt_id == command.attempt_id,
                )
            )
            if model is None:
                raise RuntimeError("conflicting usage attempt is not visible")
            self._require_same_attempt(model, command)
        if model is None:
            raise RuntimeError("inserted usage attempt is not visible")
        return self._attempt_view(model)

    def finish_attempt(
        self,
        *,
        owner_id: UUID,
        attempt_id: UUID,
        outcome: UsageOutcome,
        finished_at: datetime,
    ) -> UsageAttemptView:
        if outcome is UsageOutcome.STARTED:
            raise ValueError("finish outcome must be terminal")
        if finished_at.tzinfo is None:
            raise ValueError("finished_at must be timezone-aware")

        self._session.rollback()
        with self._session.begin():
            return self.finish_attempt_in_transaction(
                owner_id=owner_id,
                attempt_id=attempt_id,
                outcome=outcome,
                finished_at=finished_at,
            )

    def finish_attempt_in_transaction(
        self,
        *,
        owner_id: UUID,
        attempt_id: UUID,
        outcome: UsageOutcome,
        finished_at: datetime,
    ) -> UsageAttemptView:
        """Finish a usage attempt inside an existing outer transaction."""
        if outcome is UsageOutcome.STARTED:
            raise ValueError("finish outcome must be terminal")
        if finished_at.tzinfo is None:
            raise ValueError("finished_at must be timezone-aware")
        model = self._session.scalar(
            select(ResourceUsageAttempt)
            .where(
                ResourceUsageAttempt.owner_id == owner_id,
                ResourceUsageAttempt.attempt_id == attempt_id,
            )
            .with_for_update()
        )
        if model is None:
            raise ComponentPolicyUnavailableError("usage attempt is not available")
        if model.outcome == UsageOutcome.STARTED.value:
            if finished_at < model.started_at:
                raise ValueError("finished_at cannot precede started_at")
            model.outcome = outcome.value
            model.finished_at = finished_at
        elif model.outcome != outcome.value:
            raise UsageConflictError("attempt already has another terminal outcome")
        return self._attempt_view(model)

    def recover_abandoned_attempts_in_transaction(
        self,
        *,
        owner_id: UUID,
        operation_id: UUID,
        component_key: str,
        stage: str,
        finished_at: datetime,
    ) -> tuple[UUID, ...]:
        """Conservatively settle calls left open by an expired execution epoch."""
        if finished_at.tzinfo is None:
            raise ValueError("finished_at must be timezone-aware")
        attempt_ids = list(
            self._session.scalars(
                select(ResourceUsageAttempt.attempt_id)
                .join(
                    ResourceComponentPolicy,
                    (ResourceComponentPolicy.owner_id == ResourceUsageAttempt.owner_id)
                    & (ResourceComponentPolicy.id == ResourceUsageAttempt.component_policy_id),
                )
                .where(
                    ResourceUsageAttempt.owner_id == owner_id,
                    ResourceUsageAttempt.operation_id == operation_id,
                    ResourceComponentPolicy.component_key == component_key,
                    ResourceUsageAttempt.stage == stage,
                    ResourceUsageAttempt.outcome == UsageOutcome.STARTED.value,
                )
                .order_by(ResourceUsageAttempt.started_at, ResourceUsageAttempt.id)
            )
        )
        recovered: list[UUID] = []
        for attempt_id in attempt_ids:
            reservations = self._locked_reservations(owner_id, attempt_id)
            if not reservations:
                raise RuntimeError("abandoned usage attempt has no budget reservation")
            attempt = self._session.scalar(
                select(ResourceUsageAttempt)
                .where(
                    ResourceUsageAttempt.owner_id == owner_id,
                    ResourceUsageAttempt.attempt_id == attempt_id,
                )
                .with_for_update()
            )
            if attempt is None:
                raise RuntimeError("abandoned usage attempt is not available")
            if attempt.outcome != UsageOutcome.STARTED.value:
                continue
            requested_units = reservations[0].requested_units
            if any(row.requested_units != requested_units for row in reservations):
                raise RuntimeError("abandoned budget reservations disagree on requested units")
            self.settle_budget_reservation_in_transaction(
                owner_id=owner_id,
                reservation_id=attempt_id,
                actual_units=requested_units,
            )
            self.finish_attempt_in_transaction(
                owner_id=owner_id,
                attempt_id=attempt_id,
                outcome=UsageOutcome.FAILED,
                finished_at=finished_at,
            )
            recovered.append(attempt_id)
        return tuple(recovered)

    def usage_summary(self, *, owner_id: UUID, operation_id: UUID) -> UsageSummaryView:
        outcomes = list(
            self._session.scalars(
                select(ResourceUsageAttempt.outcome).where(
                    ResourceUsageAttempt.owner_id == owner_id,
                    ResourceUsageAttempt.operation_id == operation_id,
                )
            )
        )
        self._session.rollback()
        counts = {outcome.value: outcomes.count(outcome.value) for outcome in UsageOutcome}
        return UsageSummaryView(
            owner_id=owner_id,
            operation_id=operation_id,
            total_attempts=len(outcomes),
            started_attempts=counts[UsageOutcome.STARTED.value],
            succeeded_attempts=counts[UsageOutcome.SUCCEEDED.value],
            failed_attempts=counts[UsageOutcome.FAILED.value],
            filtered_attempts=counts[UsageOutcome.FILTERED.value],
            empty_attempts=counts[UsageOutcome.EMPTY.value],
        )

    def save_budget_policy(
        self,
        *,
        owner_id: UUID,
        command: BudgetPolicyInput,
    ) -> BudgetPolicyView:
        now = self._clock()
        self._require_aware_clock(now)
        self._session.rollback()
        with self._session.begin():
            model = self._session.scalar(
                select(ResourceBudgetPolicy)
                .where(
                    ResourceBudgetPolicy.owner_id == owner_id,
                    ResourceBudgetPolicy.budget_key == command.budget_key,
                )
                .with_for_update()
            )
            inserted = False
            if model is None:
                policy_id = uuid4()
                inserted_id = self._session.scalar(
                    insert(ResourceBudgetPolicy)
                    .values(
                        id=policy_id,
                        owner_id=owner_id,
                        budget_key=command.budget_key,
                        metric=command.metric.value,
                        scope_kind=command.scope_kind.value,
                        scope_reference=command.scope_reference,
                        limit_units=command.limit_units,
                        window_seconds=command.window_seconds,
                        window_anchor_at=command.window_anchor_at,
                        enabled=command.enabled,
                        policy_version=1,
                        created_at=now,
                        updated_at=now,
                    )
                    .on_conflict_do_nothing(constraint="resource_budget_policies_owner_key")
                    .returning(ResourceBudgetPolicy.id)
                )
                inserted = inserted_id is not None
                model = (
                    self._session.get(ResourceBudgetPolicy, inserted_id)
                    if inserted_id is not None
                    else self._session.scalar(
                        select(ResourceBudgetPolicy)
                        .where(
                            ResourceBudgetPolicy.owner_id == owner_id,
                            ResourceBudgetPolicy.budget_key == command.budget_key,
                        )
                        .with_for_update()
                    )
                )
            if model is None:
                raise RuntimeError("saved budget policy is not visible")
            self._require_same_budget_structure(model, command)
            if not inserted and (
                model.limit_units != command.limit_units or model.enabled != command.enabled
            ):
                model.limit_units = command.limit_units
                model.enabled = command.enabled
                model.policy_version += 1
                model.updated_at = now
            view = self._budget_policy_view(model)
        return view

    def reserve_budget(
        self,
        *,
        owner_id: UUID,
        command: BudgetReservationInput,
    ) -> BudgetReservationDecision:
        self._session.rollback()
        with self._session.begin():
            return self.reserve_budget_in_transaction(owner_id=owner_id, command=command)

    def reserve_budget_in_transaction(
        self,
        *,
        owner_id: UUID,
        command: BudgetReservationInput,
    ) -> BudgetReservationDecision:
        """Reserve all applicable budgets inside an existing outer transaction."""
        now = self._clock()
        self._require_aware_clock(now)
        fingerprint = self._budget_context_fingerprint(command)
        existing = self._locked_reservations(owner_id, command.reservation_id)
        if existing:
            return self._reservation_replay(existing, command, fingerprint)

        policies = self._applicable_budget_policies(owner_id, command, now)
        if not any(policy.scope_kind == BudgetScopeKind.GLOBAL.value for policy in policies):
            raise BudgetPolicyUnavailableError("an active global budget policy is required")

        existing = self._locked_reservations(owner_id, command.reservation_id)
        if existing:
            return self._reservation_replay(existing, command, fingerprint)

        windows: list[tuple[ResourceBudgetPolicy, ResourceBudgetWindow, int]] = []
        for policy in policies:
            window = self._locked_current_window(policy, now)
            remaining = max(
                0,
                policy.limit_units - window.used_units - window.reserved_units,
            )
            windows.append((policy, window, remaining))

        limiting = [item for item in windows if item[2] < command.requested_units]
        if limiting:
            retry_at = (
                None
                if command.metric is BudgetMetric.CONCURRENCY_SLOT
                else max(window.window_end for _, window, _ in limiting)
            )
            return BudgetReservationDecision(
                status=BudgetDecisionStatus.DELAYED,
                reservation_id=command.reservation_id,
                operation_id=command.operation_id,
                metric=command.metric,
                requested_units=command.requested_units,
                remaining_units=min(remaining for _, _, remaining in windows),
                limiting_budget_keys=tuple(policy.budget_key for policy, _, _ in limiting),
                resume_condition=(
                    BudgetResumeCondition.CAPACITY_RELEASE
                    if command.metric is BudgetMetric.CONCURRENCY_SLOT
                    else BudgetResumeCondition.NEXT_WINDOW
                ),
                retry_at=retry_at,
            )

        mode = self._budget_mode(command.metric)
        remaining_after: list[int] = []
        for policy, window, remaining in windows:
            after = remaining - command.requested_units
            window.reserved_units += command.requested_units
            window.updated_at = now
            self._session.add(
                ResourceBudgetReservation(
                    id=uuid4(),
                    owner_id=owner_id,
                    reservation_id=command.reservation_id,
                    operation_id=command.operation_id,
                    budget_policy_id=policy.id,
                    budget_window_id=window.id,
                    policy_version=policy.policy_version,
                    limit_units=policy.limit_units,
                    metric=command.metric.value,
                    budget_mode=mode,
                    requested_units=command.requested_units,
                    actual_units=None,
                    released_units=None,
                    remaining_units_after=after,
                    context_fingerprint=fingerprint,
                    status=BudgetReservationStatus.RESERVED.value,
                    created_at=now,
                    settled_at=None,
                )
            )
            remaining_after.append(after)

        return BudgetReservationDecision(
            status=BudgetDecisionStatus.RESERVED,
            reservation_id=command.reservation_id,
            operation_id=command.operation_id,
            metric=command.metric,
            requested_units=command.requested_units,
            remaining_units=min(remaining_after),
            limiting_budget_keys=(),
            resume_condition=None,
            retry_at=None,
        )

    def settle_budget_reservation(
        self,
        *,
        owner_id: UUID,
        reservation_id: UUID,
        actual_units: int,
    ) -> BudgetSettlementView:
        self._session.rollback()
        with self._session.begin():
            return self.settle_budget_reservation_in_transaction(
                owner_id=owner_id,
                reservation_id=reservation_id,
                actual_units=actual_units,
            )

    def settle_budget_reservation_in_transaction(
        self,
        *,
        owner_id: UUID,
        reservation_id: UUID,
        actual_units: int,
    ) -> BudgetSettlementView:
        """Settle one budget reservation inside an existing outer transaction."""
        if actual_units < 0:
            raise ValueError("actual_units cannot be negative")
        now = self._clock()
        self._require_aware_clock(now)
        reservations = self._locked_reservations(owner_id, reservation_id)
        if not reservations:
            raise BudgetPolicyUnavailableError("budget reservation does not exist")

        first = reservations[0]
        if actual_units > first.requested_units:
            raise ValueError("actual_units cannot exceed requested_units")
        if any(row.requested_units != first.requested_units for row in reservations):
            raise RuntimeError("budget reservation rows disagree on requested units")

        statuses = {row.status for row in reservations}
        if statuses == {BudgetReservationStatus.SETTLED.value}:
            if any(row.actual_units != actual_units for row in reservations):
                raise BudgetReservationConflictError("reservation already has another settlement")
            if first.settled_at is None or first.actual_units is None:
                raise RuntimeError("settled reservation is incomplete")
            return self._settlement_view(reservations, first.settled_at)
        if statuses != {BudgetReservationStatus.RESERVED.value}:
            raise RuntimeError("budget reservation rows have inconsistent status")

        window_by_id: dict[UUID, ResourceBudgetWindow] = {}
        for row in reservations:
            window = self._session.scalar(
                select(ResourceBudgetWindow)
                .where(
                    ResourceBudgetWindow.owner_id == owner_id,
                    ResourceBudgetWindow.id == row.budget_window_id,
                )
                .with_for_update()
            )
            if window is None:
                raise RuntimeError("budget reservation window is missing")
            window_by_id[window.id] = window

        for row in reservations:
            window = window_by_id[row.budget_window_id]
            if window.reserved_units < row.requested_units:
                raise RuntimeError("budget window reserved units are inconsistent")
            window.reserved_units -= row.requested_units
            if row.budget_mode == "cumulative":
                window.used_units += actual_units
                released_units = row.requested_units - actual_units
            else:
                released_units = row.requested_units
            window.updated_at = now
            row.actual_units = actual_units
            row.released_units = released_units
            row.status = BudgetReservationStatus.SETTLED.value
            row.settled_at = now

        return self._settlement_view(reservations, now)

    def _applicable_budget_policies(
        self,
        owner_id: UUID,
        command: BudgetReservationInput,
        now: datetime,
    ) -> list[ResourceBudgetPolicy]:
        scope_conditions = [ResourceBudgetPolicy.scope_kind == BudgetScopeKind.GLOBAL.value]
        for scope_kind, reference in (
            (BudgetScopeKind.SOURCE, command.context.source_ref),
            (BudgetScopeKind.CONNECTION, command.context.connection_ref),
            (BudgetScopeKind.JOB, command.context.job_ref),
        ):
            if reference is not None:
                scope_conditions.append(
                    (ResourceBudgetPolicy.scope_kind == scope_kind.value)
                    & (ResourceBudgetPolicy.scope_reference == reference)
                )
        return list(
            self._session.scalars(
                select(ResourceBudgetPolicy)
                .where(
                    ResourceBudgetPolicy.owner_id == owner_id,
                    ResourceBudgetPolicy.metric == command.metric.value,
                    ResourceBudgetPolicy.enabled.is_(True),
                    ResourceBudgetPolicy.window_anchor_at <= now,
                    or_(*scope_conditions),
                )
                .order_by(ResourceBudgetPolicy.id)
                .with_for_update()
            )
        )

    def _locked_current_window(
        self,
        policy: ResourceBudgetPolicy,
        now: datetime,
    ) -> ResourceBudgetWindow:
        elapsed_seconds = (now - policy.window_anchor_at).total_seconds()
        window_index = int(elapsed_seconds // policy.window_seconds)
        window_start = policy.window_anchor_at + timedelta(
            seconds=window_index * policy.window_seconds
        )
        window_end = window_start + timedelta(seconds=policy.window_seconds)
        window = self._session.scalar(
            select(ResourceBudgetWindow)
            .where(
                ResourceBudgetWindow.owner_id == policy.owner_id,
                ResourceBudgetWindow.budget_policy_id == policy.id,
                ResourceBudgetWindow.window_start == window_start,
            )
            .with_for_update()
        )
        if window is not None:
            return window

        window_id = uuid4()
        inserted_id = self._session.scalar(
            insert(ResourceBudgetWindow)
            .values(
                id=window_id,
                owner_id=policy.owner_id,
                budget_policy_id=policy.id,
                budget_mode=self._budget_mode(BudgetMetric(policy.metric)),
                window_start=window_start,
                window_end=window_end,
                used_units=0,
                reserved_units=0,
                created_at=now,
                updated_at=now,
            )
            .on_conflict_do_nothing(constraint="resource_budget_windows_policy_start_key")
            .returning(ResourceBudgetWindow.id)
        )
        window = (
            self._session.get(ResourceBudgetWindow, inserted_id)
            if inserted_id is not None
            else self._session.scalar(
                select(ResourceBudgetWindow)
                .where(
                    ResourceBudgetWindow.owner_id == policy.owner_id,
                    ResourceBudgetWindow.budget_policy_id == policy.id,
                    ResourceBudgetWindow.window_start == window_start,
                )
                .with_for_update()
            )
        )
        if window is None:
            raise RuntimeError("budget window is not visible")
        return window

    def _locked_reservations(
        self,
        owner_id: UUID,
        reservation_id: UUID,
    ) -> list[ResourceBudgetReservation]:
        return list(
            self._session.scalars(
                select(ResourceBudgetReservation)
                .where(
                    ResourceBudgetReservation.owner_id == owner_id,
                    ResourceBudgetReservation.reservation_id == reservation_id,
                )
                .order_by(ResourceBudgetReservation.budget_policy_id)
                .with_for_update()
            )
        )

    def _reservation_replay(
        self,
        reservations: list[ResourceBudgetReservation],
        command: BudgetReservationInput,
        fingerprint: bytes,
    ) -> BudgetReservationDecision:
        if any(
            row.operation_id != command.operation_id
            or row.metric != command.metric.value
            or row.requested_units != command.requested_units
            or row.context_fingerprint != fingerprint
            for row in reservations
        ):
            raise BudgetReservationConflictError("reservation identifier already has other data")
        return BudgetReservationDecision(
            status=BudgetDecisionStatus.RESERVED,
            reservation_id=command.reservation_id,
            operation_id=command.operation_id,
            metric=command.metric,
            requested_units=command.requested_units,
            remaining_units=min(row.remaining_units_after for row in reservations),
            limiting_budget_keys=(),
            resume_condition=None,
            retry_at=None,
        )

    @staticmethod
    def _budget_context_fingerprint(command: BudgetReservationInput) -> bytes:
        canonical = json.dumps(
            command.context.model_dump(mode="json"),
            ensure_ascii=True,
            separators=(",", ":"),
            sort_keys=True,
        )
        return hashlib.sha256(canonical.encode()).digest()

    @staticmethod
    def _budget_mode(metric: BudgetMetric) -> str:
        return "concurrent" if metric is BudgetMetric.CONCURRENCY_SLOT else "cumulative"

    @staticmethod
    def _require_same_budget_structure(
        model: ResourceBudgetPolicy,
        command: BudgetPolicyInput,
    ) -> None:
        if (
            model.metric != command.metric.value
            or model.scope_kind != command.scope_kind.value
            or model.scope_reference != command.scope_reference
            or model.window_seconds != command.window_seconds
            or model.window_anchor_at != command.window_anchor_at
        ):
            raise BudgetPolicyConflictError(
                "budget policy structural fields require a new budget_key"
            )

    @staticmethod
    def _require_aware_clock(now: datetime) -> None:
        if now.tzinfo is None:
            raise ValueError("budget clock must be timezone-aware")

    @staticmethod
    def _budget_policy_view(model: ResourceBudgetPolicy) -> BudgetPolicyView:
        return BudgetPolicyView(
            id=model.id,
            owner_id=model.owner_id,
            budget_key=model.budget_key,
            metric=model.metric,
            scope_kind=model.scope_kind,
            scope_reference=model.scope_reference,
            limit_units=model.limit_units,
            window_seconds=model.window_seconds,
            window_anchor_at=model.window_anchor_at,
            enabled=model.enabled,
            policy_version=model.policy_version,
            created_at=model.created_at,
            updated_at=model.updated_at,
        )

    @staticmethod
    def _settlement_view(
        reservations: list[ResourceBudgetReservation],
        settled_at: datetime,
    ) -> BudgetSettlementView:
        first = reservations[0]
        if first.actual_units is None or first.released_units is None:
            raise RuntimeError("budget reservation is not settled")
        return BudgetSettlementView(
            reservation_id=first.reservation_id,
            operation_id=first.operation_id,
            metric=first.metric,
            requested_units=first.requested_units,
            actual_units=first.actual_units,
            released_units=first.released_units,
            policy_count=len(reservations),
            settled_at=settled_at,
        )

    def _require_same_attempt(
        self,
        model: ResourceUsageAttempt,
        command: UsageAttemptInput,
    ) -> None:
        component_key = self._session.scalar(
            select(ResourceComponentPolicy.component_key).where(
                ResourceComponentPolicy.owner_id == model.owner_id,
                ResourceComponentPolicy.id == model.component_policy_id,
            )
        )
        if (
            component_key != command.component_key
            or model.operation_id != command.operation_id
            or model.usage_kind != command.usage_kind.value
            or model.stage != command.stage
        ):
            raise UsageConflictError("attempt identifier already has other data")

    @staticmethod
    def _component_policy_view(model: ResourceComponentPolicy) -> ComponentPolicyView:
        return ComponentPolicyView(
            id=model.id,
            owner_id=model.owner_id,
            component_key=model.component_key,
            component_version=model.component_version,
            cost_class=CostClass(model.cost_class),
            enabled_for_core=model.enabled_for_core,
            terms_reference=model.terms_reference,
            reviewed_at=model.reviewed_at,
            policy_version=model.policy_version,
            created_at=model.created_at,
            updated_at=model.updated_at,
        )

    @staticmethod
    def _attempt_view(model: ResourceUsageAttempt) -> UsageAttemptView:
        return UsageAttemptView(
            id=model.id,
            owner_id=model.owner_id,
            attempt_id=model.attempt_id,
            operation_id=model.operation_id,
            component_policy_id=model.component_policy_id,
            component_version=model.component_version,
            usage_kind=model.usage_kind,
            stage=model.stage,
            outcome=model.outcome,
            started_at=model.started_at,
            finished_at=model.finished_at,
        )


class JobObservationService:
    def __init__(self, session: Session) -> None:
        self._session = session

    def start_stage(
        self,
        *,
        owner_id: UUID,
        job_id: UUID,
        command: StageAttemptInput,
    ) -> StageAttemptView:
        self._session.rollback()
        with self._session.begin():
            job = self._session.scalar(
                select(Job).where(Job.owner_id == owner_id, Job.id == job_id)
            )
            if job is None:
                raise StageAttemptUnavailableError("job is not available")
            if command.started_at < job.created_at:
                raise ValueError("stage attempt cannot start before its job")

            inserted_id = self._session.scalar(
                insert(JobStageAttempt)
                .values(
                    id=command.attempt_id,
                    owner_id=owner_id,
                    job_id=job_id,
                    stage=command.stage.value,
                    attempt_sequence=command.attempt_sequence,
                    outcome=JobStageOutcome.STARTED.value,
                    started_at=command.started_at,
                    finished_at=None,
                )
                .on_conflict_do_nothing()
                .returning(JobStageAttempt.id)
            )
            if inserted_id is not None:
                model = self._session.get(JobStageAttempt, inserted_id)
            else:
                model = self._session.get(JobStageAttempt, command.attempt_id)
                if model is None:
                    model = self._session.scalar(
                        select(JobStageAttempt).where(
                            JobStageAttempt.job_id == job_id,
                            JobStageAttempt.stage == command.stage.value,
                            JobStageAttempt.attempt_sequence == command.attempt_sequence,
                        )
                    )
            if model is None or (
                model.id != command.attempt_id
                or model.owner_id != owner_id
                or model.job_id != job_id
                or model.stage != command.stage.value
                or model.attempt_sequence != command.attempt_sequence
                or model.started_at != command.started_at
            ):
                raise StageAttemptConflictError("stage attempt was replayed with other facts")
            view = self._stage_view(model)
        return view

    def finish_stage(
        self,
        *,
        owner_id: UUID,
        attempt_id: UUID,
        outcome: JobStageOutcome,
        finished_at: datetime,
    ) -> StageAttemptView:
        if outcome is JobStageOutcome.STARTED:
            raise ValueError("finish outcome must be terminal")
        if finished_at.tzinfo is None:
            raise ValueError("finished_at must be timezone-aware")

        self._session.rollback()
        with self._session.begin():
            model = self._session.scalar(
                select(JobStageAttempt)
                .where(
                    JobStageAttempt.owner_id == owner_id,
                    JobStageAttempt.id == attempt_id,
                )
                .with_for_update()
            )
            if model is None:
                raise StageAttemptUnavailableError("stage attempt is not available")
            if model.outcome == JobStageOutcome.STARTED.value:
                if finished_at < model.started_at:
                    raise ValueError("finished_at cannot precede started_at")
                model.outcome = outcome.value
                model.finished_at = finished_at
            elif model.outcome != outcome.value or model.finished_at != finished_at:
                raise StageAttemptConflictError("stage attempt already has other terminal facts")
            view = self._stage_view(model)
        return view

    def snapshot(
        self,
        *,
        owner_id: UUID,
        window_start: datetime,
        window_end: datetime,
    ) -> OperationalSnapshot:
        if window_start.tzinfo is None or window_end.tzinfo is None:
            raise ValueError("observation window must be timezone-aware")
        if window_end <= window_start:
            raise ValueError("observation window must be ordered")

        jobs = list(
            self._session.scalars(
                select(Job)
                .where(
                    Job.owner_id == owner_id,
                    Job.created_at >= window_start,
                    Job.created_at < window_end,
                )
                .order_by(Job.created_at, Job.id)
            )
        )
        job_ids = [job.id for job in jobs]
        operation_ids = sorted({job.operation_id for job in jobs}, key=str)
        execution_counts: dict[UUID, int] = {}
        stage_counts: dict[UUID, int] = {}
        resource_counts: dict[UUID, int] = {}
        if job_ids:
            execution_counts = {
                job_id: int(count)
                for job_id, count in self._session.execute(
                    select(JobAttempt.job_id, func.count())
                    .where(JobAttempt.job_id.in_(job_ids))
                    .group_by(JobAttempt.job_id)
                )
            }
            stage_counts = {
                job_id: int(count)
                for job_id, count in self._session.execute(
                    select(JobStageAttempt.job_id, func.count())
                    .where(JobStageAttempt.job_id.in_(job_ids))
                    .group_by(JobStageAttempt.job_id)
                )
            }
        if operation_ids:
            resource_counts = {
                operation_id: int(count)
                for operation_id, count in self._session.execute(
                    select(ResourceUsageAttempt.operation_id, func.count())
                    .where(
                        ResourceUsageAttempt.owner_id == owner_id,
                        ResourceUsageAttempt.operation_id.in_(operation_ids),
                    )
                    .group_by(ResourceUsageAttempt.operation_id)
                )
            }
        task_counts = {status: 0 for status in OperationalTaskStatus}
        records: list[OperationalTaskRecord] = []
        for job in jobs:
            status = self._operational_status(job)
            task_counts[status] += 1
            records.append(
                OperationalTaskRecord(
                    job_id=job.id,
                    operation_id=job.operation_id,
                    kind=job.kind,
                    observation=self._observation(job),
                    status=status,
                    scheduled_for_at=job.scheduled_for_at,
                    created_at=job.created_at,
                    started_at=job.started_at,
                    completed_at=job.completed_at,
                    next_run_at=job.next_run_at,
                    execution_attempts=execution_counts.get(job.id, 0),
                    stage_attempts=stage_counts.get(job.id, 0),
                )
            )
        operations = tuple(
            OperationAttemptCount(
                operation_id=operation_id,
                attempts=resource_counts.get(operation_id, 0),
            )
            for operation_id in operation_ids
        )
        summary = OperationalSummary(
            total_tasks=len(records),
            task_counts=task_counts,
            execution_attempts=sum(execution_counts.values()),
            stage_attempts=sum(stage_counts.values()),
            resource_attempts=sum(resource_counts.values()),
        )
        self._session.rollback()
        return OperationalSnapshot(
            owner_id=owner_id,
            window_start=window_start,
            window_end=window_end,
            tasks=tuple(records),
            operations=operations,
            summary=summary,
        )

    @staticmethod
    def _operational_status(model: Job) -> OperationalTaskStatus:
        if model.status == JobStatus.QUEUED.value and model.next_run_at is not None:
            return OperationalTaskStatus.DELAYED
        return OperationalTaskStatus(model.status)

    @staticmethod
    def _observation(model: Job) -> JobObservationContext:
        return JobObservationContext(
            configuration_ref=model.configuration_ref,
            configuration_version=model.configuration_version,
            source_key=model.source_key,
            source_capability=(
                SourceCapability(model.source_capability)
                if model.source_capability is not None
                else None
            ),
        )

    @staticmethod
    def _stage_view(model: JobStageAttempt) -> StageAttemptView:
        return StageAttemptView(
            attempt_id=model.id,
            owner_id=model.owner_id,
            job_id=model.job_id,
            stage=JobStage(model.stage),
            attempt_sequence=model.attempt_sequence,
            outcome=JobStageOutcome(model.outcome),
            started_at=model.started_at,
            finished_at=model.finished_at,
        )


class JobService:
    def __init__(
        self,
        session: Session,
        *,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def accept(self, *, owner_id: UUID, command: JobAcceptanceInput) -> JobView:
        self._session.rollback()
        with self._session.begin():
            return self.accept_in_transaction(owner_id=owner_id, command=command)

    def accept_in_transaction(
        self,
        *,
        owner_id: UUID,
        command: JobAcceptanceInput,
    ) -> JobView:
        """Persist a job and its first outbox event in an existing transaction."""
        fingerprint = fingerprint_request(command)
        now = self._clock()
        if now.tzinfo is None:
            raise ValueError("clock must return a timezone-aware datetime")
        job_id = uuid4()
        inserted_id = self._session.scalar(
            insert(Job)
            .values(
                id=job_id,
                owner_id=owner_id,
                operation_id=command.operation_id,
                kind=command.kind,
                configuration_ref=command.observation.configuration_ref,
                configuration_version=command.observation.configuration_version,
                source_key=command.observation.source_key,
                source_capability=(
                    command.observation.source_capability.value
                    if command.observation.source_capability is not None
                    else None
                ),
                scope=command.scope,
                request_fingerprint=fingerprint,
                status=JobStatus.QUEUED.value,
                scheduled_for_at=command.scheduled_for_at,
                created_at=now,
                updated_at=now,
            )
            .on_conflict_do_nothing(constraint="jobs_owner_kind_operation_key")
            .returning(Job.id)
        )

        if inserted_id is not None:
            require_source_connection_enabled(
                self._session,
                owner_id=owner_id,
                source_key=command.observation.source_key,
            )
            self._session.add(
                OutboxMessage(
                    id=uuid4(),
                    aggregate_id=job_id,
                    topic=JOB_ACCEPTED_TOPIC,
                    message_key=job_id,
                    event_type=JOB_ACCEPTED_EVENT_TYPE,
                    dispatch_sequence=1,
                    payload={
                        "job_id": str(job_id),
                        "owner_id": str(owner_id),
                        "operation_id": str(command.operation_id),
                        "kind": command.kind,
                        "configuration_ref": command.observation.configuration_ref,
                        "configuration_version": command.observation.configuration_version,
                        "source_key": command.observation.source_key,
                        "source_capability": (
                            command.observation.source_capability.value
                            if command.observation.source_capability is not None
                            else None
                        ),
                    },
                    available_at=now,
                    created_at=now,
                    published_at=None,
                )
            )
            return JobView(
                id=job_id,
                owner_id=owner_id,
                operation_id=command.operation_id,
                kind=command.kind,
                observation=command.observation,
                status=JobStatus.QUEUED,
                scheduled_for_at=command.scheduled_for_at,
                started_at=None,
                completed_at=None,
                created_at=now,
            )

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
        return self._view(existing)

    def get_status(self, *, owner_id: UUID, job_id: UUID) -> JobStatusView:
        now = self._clock()
        if now.tzinfo is None:
            raise ValueError("clock must return a timezone-aware datetime")
        model = self._session.scalar(
            select(Job).where(
                Job.owner_id == owner_id,
                Job.id == job_id,
            )
        )
        if model is None:
            self._session.rollback()
            raise ApplicationError("resource_not_found")
        try:
            return self._status_view(model, now=now)
        finally:
            self._session.rollback()

    def request_cancel(self, *, owner_id: UUID, job_id: UUID) -> JobStatusView:
        now = self._clock()
        if now.tzinfo is None:
            raise ValueError("clock must return a timezone-aware datetime")
        self._session.rollback()
        with self._session.begin():
            model = self._session.scalar(
                select(Job).where(Job.owner_id == owner_id, Job.id == job_id).with_for_update()
            )
            if model is None:
                raise ApplicationError("resource_not_found")

            if model.status == JobStatus.QUEUED.value:
                model.status = JobStatus.CANCELLED.value
                model.cancel_requested_at = now
                model.completed_at = now
                model.defer_reason = None
                model.next_run_at = None
                model.updated_at = now
            elif model.status == JobStatus.RUNNING.value:
                if model.cancel_requested_at is None:
                    if model.lease_expires_at is None:
                        raise RuntimeError("running job is missing its execution lease")
                    model.cancel_requested_at = now
                    if model.lease_expires_at <= now:
                        model.status = JobStatus.CANCELLED.value
                        model.cancel_deadline_at = now
                        model.lease_owner = None
                        model.lease_expires_at = None
                        model.completed_at = now
                        self._session.execute(
                            update(JobAttempt)
                            .where(
                                JobAttempt.job_id == model.id,
                                JobAttempt.lease_epoch == model.lease_epoch,
                                JobAttempt.finished_at.is_(None),
                            )
                            .values(finished_at=now, outcome="cancelled")
                        )
                    else:
                        model.cancel_deadline_at = model.lease_expires_at
                    model.updated_at = now
            elif model.status != JobStatus.CANCELLED.value:
                raise ApplicationError("job_not_cancellable")

            view = self._status_view(model, now=now)
        return view

    def request_retry(self, *, owner_id: UUID, job_id: UUID) -> JobStatusView:
        now = self._clock()
        if now.tzinfo is None:
            raise ValueError("clock must return a timezone-aware datetime")
        self._session.rollback()
        with self._session.begin():
            model = self._session.scalar(
                select(Job).where(Job.owner_id == owner_id, Job.id == job_id).with_for_update()
            )
            if model is None:
                raise ApplicationError("resource_not_found")

            if (
                model.status == JobStatus.QUEUED.value
                and model.defer_reason == "manual_retry"
                and model.last_error_at is not None
            ):
                return self._status_view(model, now=now)
            if model.status != JobStatus.FAILED.value or not model.manual_retry_allowed:
                raise ApplicationError("job_not_retryable")

            require_source_connection_enabled(
                self._session, owner_id=owner_id, source_key=model.source_key
            )
            retry_count = model.retry_count + 1
            dispatch_sequence = (
                self._session.scalar(
                    select(func.max(OutboxMessage.dispatch_sequence)).where(
                        OutboxMessage.aggregate_id == model.id
                    )
                )
                or 0
            ) + 1
            model.status = JobStatus.QUEUED.value
            model.completed_at = None
            model.defer_reason = "manual_retry"
            model.next_run_at = now
            model.retry_count = retry_count
            model.updated_at = now
            self._session.add(
                OutboxMessage(
                    id=uuid4(),
                    aggregate_id=model.id,
                    topic=JOB_ACCEPTED_TOPIC,
                    message_key=model.id,
                    event_type=JOB_RETRY_EVENT_TYPE,
                    dispatch_sequence=dispatch_sequence,
                    payload=self._retry_payload(
                        model,
                        retry_at=now,
                        dispatch_sequence=dispatch_sequence,
                    ),
                    available_at=now,
                    created_at=now,
                    published_at=None,
                )
            )
            return self._status_view(model, now=now)

    def accept_schedule_window(
        self,
        *,
        owner_id: UUID,
        kind: str,
        schedule_key: str,
        window: ScheduleWindow,
        observation: JobObservationContext,
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
                observation=observation,
                scheduled_for_at=window.end,
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
            observation=JobObservationService._observation(model),
            status=JobStatus(model.status),
            scheduled_for_at=model.scheduled_for_at,
            started_at=model.started_at,
            completed_at=model.completed_at,
            created_at=model.created_at,
        )

    @staticmethod
    def _status_view(model: Job, *, now: datetime) -> JobStatusView:
        public_status = (
            JobControlStatus.CANCELLING
            if model.status == JobStatus.RUNNING.value and model.cancel_requested_at is not None
            else JobControlStatus(model.status)
        )
        cancellation = None
        if model.cancel_requested_at is not None:
            deadline = model.cancel_deadline_at
            timed_out = deadline is not None and (
                (model.completed_at is None and now >= deadline)
                or (model.completed_at is not None and model.completed_at >= deadline)
            )
            cancellation = JobCancellationView(
                requested_at=model.cancel_requested_at.astimezone(UTC),
                deadline_at=_as_utc(deadline),
                timed_out=timed_out,
            )
        failure = None
        if model.last_error_code is not None:
            if (
                model.last_error_category is None
                or model.last_error_at is None
                or model.next_action is None
            ):
                raise RuntimeError("job failure context is incomplete")
            failure = JobFailureView(
                error_code=model.last_error_code,
                category=JobFailureCategory(model.last_error_category),
                occurred_at=model.last_error_at.astimezone(UTC),
                next_action=model.next_action,
                manual_retry_allowed=model.manual_retry_allowed,
            )
        result_content_id = None
        raw_content_id = model.checkpoint.get("content_id")
        if raw_content_id is not None:
            if not isinstance(raw_content_id, str):
                raise RuntimeError("job result content ID is invalid")
            try:
                result_content_id = UUID(raw_content_id)
            except ValueError as error:
                raise RuntimeError("job result content ID is invalid") from error
        return JobStatusView(
            id=model.id,
            operation_id=model.operation_id,
            kind=model.kind,
            observation=JobObservationService._observation(model),
            status=public_status,
            progress=JobProgressView(
                stage=JobStage(model.progress_stage) if model.progress_stage is not None else None,
                requests_sent=model.requests_sent,
                items_saved=model.items_saved,
                updated_at=_as_utc(model.progress_updated_at),
            ),
            cancellation=cancellation,
            failure=failure,
            result_content_id=result_content_id,
            retry_count=model.retry_count,
            next_run_at=_as_utc(model.next_run_at),
            scheduled_for_at=_as_utc(model.scheduled_for_at),
            started_at=_as_utc(model.started_at),
            completed_at=_as_utc(model.completed_at),
            created_at=model.created_at.astimezone(UTC),
        )

    @staticmethod
    def _retry_payload(
        model: Job,
        *,
        retry_at: datetime,
        dispatch_sequence: int,
    ) -> dict[str, OutboxValue]:
        if model.last_error_code is None:
            raise RuntimeError("retryable job is missing its last error code")
        return {
            "job_id": str(model.id),
            "owner_id": str(model.owner_id),
            "operation_id": str(model.operation_id),
            "kind": model.kind,
            "configuration_ref": model.configuration_ref,
            "configuration_version": model.configuration_version,
            "source_key": model.source_key,
            "source_capability": model.source_capability,
            "dispatch_sequence": dispatch_sequence,
            "retry_count": model.retry_count,
            "retry_at": retry_at.astimezone(UTC).isoformat().replace("+00:00", "Z"),
            "last_error_code": model.last_error_code,
        }


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
        if now.tzinfo is None:
            raise ValueError("published_at must be timezone-aware")
        self._session.rollback()
        with self._session.begin():
            messages = list(
                self._session.scalars(
                    select(OutboxMessage)
                    .where(
                        OutboxMessage.published_at.is_(None),
                        OutboxMessage.available_at <= now,
                    )
                    .order_by(OutboxMessage.available_at, OutboxMessage.id)
                    .limit(batch_size)
                    .with_for_update(skip_locked=True)
                )
            )
            for model in messages:
                try:
                    schema_version = JOB_EVENT_SCHEMA_VERSIONS[model.event_type]
                except KeyError as error:
                    raise RuntimeError(f"unsupported job event type: {model.event_type}") from error
                publish(
                    OutboxEnvelope(
                        message_id=model.id,
                        topic=model.topic,
                        message_key=model.message_key,
                        event_type=model.event_type,
                        schema_version=schema_version,
                        payload=dict(model.payload),
                    )
                )
                model.published_at = now
        return len(messages)
