from __future__ import annotations

import hashlib
import json
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

from sqlalchemy import or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session

from core.errors import ApplicationError
from jobs.execution import ScheduleWindow, scheduled_operation_id
from jobs.models import (
    Job,
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
    ComponentPolicyInput,
    ComponentPolicyView,
    CostClass,
    JobAcceptanceInput,
    JobStatus,
    JobView,
    UsageAttemptInput,
    UsageAttemptView,
    UsageOutcome,
    UsageSummaryView,
)

JOB_ACCEPTED_EVENT_TYPE = "job.accepted.v1"
JOB_ACCEPTED_TOPIC = "hotkey.jobs.accepted.v1"

type PublishOutbox = Callable[["OutboxEnvelope"], None]


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
            view = self._attempt_view(model)
        return view

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
            view = self._attempt_view(model)
        return view

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
        now = self._clock()
        self._require_aware_clock(now)
        fingerprint = self._budget_context_fingerprint(command)
        self._session.rollback()
        with self._session.begin():
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
        if actual_units < 0:
            raise ValueError("actual_units cannot be negative")
        now = self._clock()
        self._require_aware_clock(now)
        self._session.rollback()
        with self._session.begin():
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
                    raise BudgetReservationConflictError(
                        "reservation already has another settlement"
                    )
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
