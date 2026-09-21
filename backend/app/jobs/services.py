from __future__ import annotations

import hashlib
import json
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID, uuid4

from sqlalchemy import or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session

from core.errors import ApplicationError
from jobs.execution import ScheduleWindow, scheduled_operation_id
from jobs.models import Job, OutboxMessage, ResourceComponentPolicy, ResourceUsageAttempt
from jobs.schemas import (
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

    def __init__(self, session: Session) -> None:
        self._session = session

    def save_component_policy(
        self,
        *,
        owner_id: UUID,
        command: ComponentPolicyInput,
    ) -> ComponentPolicyView:
        now = datetime.now(UTC)
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
            view = self._policy_view(model)
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
    def _policy_view(model: ResourceComponentPolicy) -> ComponentPolicyView:
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
