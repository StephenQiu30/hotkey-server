from __future__ import annotations

import math
import re
from collections.abc import Callable, Mapping
from copy import deepcopy
from datetime import UTC, datetime, timedelta
from typing import TypeGuard, cast
from uuid import UUID, uuid4

from sqlalchemy import or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from evidence.models import (
    CleanupTarget,
    DeletionDirective,
    EvidenceResource,
    RetentionPolicy,
    SourceAccessPolicy,
)
from evidence.schemas import (
    AccessBasis,
    AccessPolicyStatus,
    AdmittedScalar,
    AdmittedSourcePayload,
    AdmittedValue,
    CleanupBatchResult,
    CleanupLease,
    CleanupStatus,
    CleanupTargetKind,
    CleanupTargetSpec,
    DataClass,
    DeletionReason,
    DeletionStatus,
    DeletionView,
    EvidenceResourceView,
    RetentionPolicyInput,
    RetentionPolicyView,
    SourceAccessPolicyInput,
    SourceAccessPolicyView,
)
from sources.contracts import SourceCapability

type Clock = Callable[[], datetime]


class SourceAccessUnavailableError(RuntimeError):
    """The requested source capability is not currently approved and enabled."""


class RetentionPolicyUnavailableError(RuntimeError):
    """The source data class has no current persistence policy."""


class ResourceUnavailableError(RuntimeError):
    """The resource is not registered, has expired, or is deletion-blocked."""


class LifecycleConflictError(RuntimeError):
    """An idempotency key or resource identity conflicts with an existing record."""


class StaleCleanupLeaseError(RuntimeError):
    """A cleanup completion attempted to use an expired or replaced lease."""


class CleanupHandlerUnavailableError(RuntimeError):
    """No concrete online-store cleanup handler is registered for the target."""


ONLINE_CLEANUP_SLA = timedelta(hours=24)
CLEANUP_LEASE_DURATION = timedelta(minutes=5)
MAX_CLEANUP_ATTEMPTS = 6


def _is_admitted_scalar(value: object) -> TypeGuard[AdmittedScalar]:
    return value is None or isinstance(value, (str, int, float, bool))


def _copy_admitted_value(value: object) -> AdmittedValue:
    if _is_admitted_scalar(value):
        if isinstance(value, float) and not math.isfinite(value):
            raise ValueError("admitted numeric fields must be finite")
        return value
    if isinstance(value, list) and all(_is_admitted_scalar(item) for item in value):
        scalar_values = cast(list[AdmittedScalar], value)
        if any(isinstance(item, float) and not math.isfinite(item) for item in scalar_values):
            raise ValueError("admitted numeric fields must be finite")
        return deepcopy(scalar_values)
    raise ValueError("admitted fields must be JSON scalars or scalar lists")


def minimize_payload(
    *,
    field_purposes: Mapping[str, str],
    payload: Mapping[str, object],
) -> dict[str, AdmittedValue]:
    minimized: dict[str, AdmittedValue] = {}
    for field_name in field_purposes:
        if field_name in payload:
            minimized[field_name] = _copy_admitted_value(payload[field_name])
    return minimized


def effective_retention_days(command: RetentionPolicyInput) -> int:
    if command.source_max_days is None:
        return command.requested_days
    return min(command.requested_days, command.source_max_days)


def retry_delay(attempt_count: int) -> timedelta:
    if attempt_count < 1:
        raise ValueError("attempt_count must be positive")
    return timedelta(seconds=min(60 * (2 ** (attempt_count - 1)), 3600))


class SourceAccessPolicyService:
    def __init__(self, session: Session, *, clock: Clock | None = None) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def save(
        self,
        *,
        owner_id: UUID,
        command: SourceAccessPolicyInput,
    ) -> SourceAccessPolicyView:
        now = self._clock()
        if command.enabled and (
            command.reviewed_at is None
            or command.reviewed_at > now
            or (command.review_expires_at is not None and command.review_expires_at <= now)
        ):
            raise ValueError("enabled policy review must be current")

        values = {
            "status": command.status.value,
            "enabled": command.enabled,
            "access_basis": command.access_basis.value if command.access_basis else None,
            "terms_reference": command.terms_reference,
            "processing_purpose": command.processing_purpose,
            "component_name": command.component_name,
            "component_version": command.component_version,
            "component_license": command.component_license,
            "field_purposes": command.field_purposes,
            "reviewed_at": command.reviewed_at,
            "review_expires_at": command.review_expires_at,
            "updated_at": now,
        }
        statement = (
            insert(SourceAccessPolicy)
            .values(
                id=uuid4(),
                owner_id=owner_id,
                source_key=command.source_key,
                capability=command.capability.value,
                policy_version=1,
                created_at=now,
                **values,
            )
            .on_conflict_do_update(
                constraint="source_access_policies_owner_source_capability_key",
                set_={
                    **values,
                    "policy_version": SourceAccessPolicy.policy_version + 1,
                },
            )
            .returning(SourceAccessPolicy)
        )
        self._session.rollback()
        with self._session.begin():
            model = self._session.scalar(statement)
            if model is None:
                raise RuntimeError("source access policy was not returned")
            view = self._view(model)
        return view

    def get(
        self,
        *,
        owner_id: UUID,
        source_key: str,
        capability: SourceCapability,
    ) -> SourceAccessPolicyView:
        model = self._find(owner_id, source_key, capability)
        if model is None:
            self._session.rollback()
            raise SourceAccessUnavailableError("source access policy is unavailable")
        try:
            return self._view(model)
        finally:
            self._session.rollback()

    def admit_payload(
        self,
        *,
        owner_id: UUID,
        source_key: str,
        capability: SourceCapability,
        data_class: DataClass,
        collected_at: datetime,
        payload: Mapping[str, object],
    ) -> AdmittedSourcePayload:
        model = self._find(owner_id, source_key, capability)
        now = self._clock()
        if (
            model is None
            or model.status != AccessPolicyStatus.APPROVED.value
            or not model.enabled
            or not model.field_purposes
            or (model.review_expires_at is not None and model.review_expires_at <= now)
        ):
            self._session.rollback()
            raise SourceAccessUnavailableError("source access policy is unavailable")
        if collected_at.tzinfo is None or collected_at > now:
            self._session.rollback()
            raise ValueError("collected_at must be timezone-aware and cannot be in the future")
        retention = self._session.scalar(
            select(RetentionPolicy).where(
                RetentionPolicy.owner_id == owner_id,
                RetentionPolicy.source_policy_id == model.id,
                RetentionPolicy.data_class == data_class.value,
            )
        )
        if (
            retention is None
            or retention.source_policy_version != model.policy_version
            or retention.effective_days == 0
        ):
            self._session.rollback()
            raise RetentionPolicyUnavailableError("retention policy is unavailable")
        policy_id = model.id
        policy_version = model.policy_version
        policy_owner_id = model.owner_id
        policy_source_key = model.source_key
        policy_capability = SourceCapability(model.capability)
        field_purposes = dict(model.field_purposes)
        retention_policy_id = retention.id
        retention_policy_version = retention.policy_version
        expires_at = collected_at + timedelta(days=retention.effective_days)
        self._session.rollback()
        fields = minimize_payload(
            field_purposes=field_purposes,
            payload=payload,
        )
        if not fields:
            raise ValueError("payload contains no admitted fields")
        admitted = AdmittedSourcePayload(
            policy_id=policy_id,
            policy_version=policy_version,
            owner_id=policy_owner_id,
            source_key=policy_source_key,
            capability=policy_capability,
            retention_policy_id=retention_policy_id,
            retention_policy_version=retention_policy_version,
            data_class=data_class,
            collected_at=collected_at,
            expires_at=expires_at,
            fields=fields,
        )
        return admitted

    def _find(
        self,
        owner_id: UUID,
        source_key: str,
        capability: SourceCapability,
    ) -> SourceAccessPolicy | None:
        return self._session.scalar(
            select(SourceAccessPolicy).where(
                SourceAccessPolicy.owner_id == owner_id,
                SourceAccessPolicy.source_key == source_key,
                SourceAccessPolicy.capability == capability.value,
            )
        )

    @staticmethod
    def _view(model: SourceAccessPolicy) -> SourceAccessPolicyView:
        return SourceAccessPolicyView(
            id=model.id,
            owner_id=model.owner_id,
            source_key=model.source_key,
            capability=SourceCapability(model.capability),
            status=AccessPolicyStatus(model.status),
            enabled=model.enabled,
            access_basis=(AccessBasis(model.access_basis) if model.access_basis else None),
            terms_reference=model.terms_reference,
            processing_purpose=model.processing_purpose,
            component_name=model.component_name,
            component_version=model.component_version,
            component_license=model.component_license,
            field_purposes=dict(model.field_purposes),
            reviewed_at=model.reviewed_at,
            review_expires_at=model.review_expires_at,
            policy_version=model.policy_version,
            created_at=model.created_at,
            updated_at=model.updated_at,
        )


class RetentionPolicyService:
    def __init__(self, session: Session, *, clock: Clock | None = None) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def save(
        self,
        *,
        owner_id: UUID,
        command: RetentionPolicyInput,
    ) -> RetentionPolicyView:
        now = self._clock()
        effective_days = effective_retention_days(command)
        self._session.rollback()
        with self._session.begin():
            source_policy = self._session.scalar(
                select(SourceAccessPolicy)
                .where(
                    SourceAccessPolicy.owner_id == owner_id,
                    SourceAccessPolicy.id == command.source_policy_id,
                )
                .with_for_update()
            )
            if source_policy is None:
                raise SourceAccessUnavailableError("source access policy is unavailable")
            model = self._session.scalar(
                select(RetentionPolicy)
                .where(
                    RetentionPolicy.owner_id == owner_id,
                    RetentionPolicy.source_policy_id == command.source_policy_id,
                    RetentionPolicy.data_class == command.data_class.value,
                )
                .with_for_update()
            )
            previous_effective_days: int | None = None
            if model is None:
                model = RetentionPolicy(
                    id=uuid4(),
                    owner_id=owner_id,
                    source_policy_id=command.source_policy_id,
                    source_policy_version=source_policy.policy_version,
                    data_class=command.data_class.value,
                    requested_days=command.requested_days,
                    source_max_days=command.source_max_days,
                    effective_days=effective_days,
                    policy_version=1,
                    created_at=now,
                    updated_at=now,
                )
                self._session.add(model)
            else:
                previous_effective_days = model.effective_days
                model.source_policy_version = source_policy.policy_version
                model.requested_days = command.requested_days
                model.source_max_days = command.source_max_days
                model.effective_days = effective_days
                model.policy_version += 1
                model.updated_at = now
            self._session.flush()
            if previous_effective_days is not None and effective_days < previous_effective_days:
                resources = self._session.scalars(
                    select(EvidenceResource)
                    .where(EvidenceResource.retention_policy_id == model.id)
                    .with_for_update()
                ).all()
                for resource in resources:
                    stricter_expiry = resource.collected_at + timedelta(days=effective_days)
                    if stricter_expiry < resource.expires_at:
                        resource.expires_at = stricter_expiry
                        resource.retention_policy_version = model.policy_version
            view = self._view(model)
        return view

    @staticmethod
    def _view(model: RetentionPolicy) -> RetentionPolicyView:
        return RetentionPolicyView(
            id=model.id,
            owner_id=model.owner_id,
            source_policy_id=model.source_policy_id,
            source_policy_version=model.source_policy_version,
            data_class=DataClass(model.data_class),
            requested_days=model.requested_days,
            source_max_days=model.source_max_days,
            effective_days=model.effective_days,
            policy_version=model.policy_version,
            created_at=model.created_at,
            updated_at=model.updated_at,
        )


class LifecycleService:
    def __init__(self, session: Session, *, clock: Clock | None = None) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def track_resource(
        self,
        *,
        owner_id: UUID,
        resource_type: str,
        resource_id: UUID,
        admission: AdmittedSourcePayload,
        cleanup_targets: list[CleanupTargetSpec],
    ) -> EvidenceResourceView:
        self._validate_resource_type(resource_type)
        target_keys = {(target.kind, target.reference) for target in cleanup_targets}
        if len(target_keys) != len(cleanup_targets):
            raise ValueError("cleanup targets must be unique")
        now = self._clock()
        if admission.owner_id != owner_id or admission.expires_at <= now:
            raise ResourceUnavailableError("resource admission is unavailable or expired")
        serialized_targets = [
            target.model_dump(mode="json")
            for target in sorted(cleanup_targets, key=lambda item: (item.kind, item.reference))
        ]
        self._session.rollback()
        with self._session.begin():
            source_policy = self._session.scalar(
                select(SourceAccessPolicy).where(
                    SourceAccessPolicy.owner_id == owner_id,
                    SourceAccessPolicy.id == admission.policy_id,
                )
            )
            retention = self._session.scalar(
                select(RetentionPolicy).where(
                    RetentionPolicy.owner_id == owner_id,
                    RetentionPolicy.id == admission.retention_policy_id,
                )
            )
            if (
                source_policy is None
                or source_policy.policy_version != admission.policy_version
                or source_policy.status != AccessPolicyStatus.APPROVED.value
                or not source_policy.enabled
                or (
                    source_policy.review_expires_at is not None
                    and source_policy.review_expires_at <= now
                )
                or retention is None
                or retention.policy_version != admission.retention_policy_version
                or retention.source_policy_id != admission.policy_id
                or retention.source_policy_version != source_policy.policy_version
                or retention.data_class != admission.data_class.value
                or retention.effective_days == 0
                or admission.expires_at
                != admission.collected_at + timedelta(days=retention.effective_days)
            ):
                raise ResourceUnavailableError("resource admission is stale")
            model = self._session.scalar(
                select(EvidenceResource)
                .where(
                    EvidenceResource.owner_id == owner_id,
                    EvidenceResource.resource_type == resource_type,
                    EvidenceResource.resource_id == resource_id,
                )
                .with_for_update()
            )
            if model is not None:
                if (
                    model.source_policy_id != admission.policy_id
                    or model.source_policy_version != admission.policy_version
                    or model.retention_policy_id != admission.retention_policy_id
                    or model.retention_policy_version != admission.retention_policy_version
                    or model.data_class != admission.data_class.value
                    or model.collected_at != admission.collected_at
                    or model.expires_at != admission.expires_at
                    or model.cleanup_targets != serialized_targets
                ):
                    raise LifecycleConflictError("resource identity is already tracked differently")
                return self._resource_view(model)
            model = EvidenceResource(
                id=uuid4(),
                owner_id=owner_id,
                resource_type=resource_type,
                resource_id=resource_id,
                source_policy_id=admission.policy_id,
                source_policy_version=admission.policy_version,
                retention_policy_id=admission.retention_policy_id,
                retention_policy_version=admission.retention_policy_version,
                data_class=admission.data_class.value,
                collected_at=admission.collected_at,
                expires_at=admission.expires_at,
                cleanup_targets=serialized_targets,
                created_at=now,
            )
            self._session.add(model)
            self._session.flush()
            view = self._resource_view(model)
        return view

    def assert_readable(
        self,
        *,
        owner_id: UUID,
        resource_type: str,
        resource_id: UUID,
    ) -> EvidenceResourceView:
        self._validate_resource_type(resource_type)
        model = self._resource_by_identity(owner_id, resource_type, resource_id)
        if model is None or model.expires_at <= self._clock():
            self._session.rollback()
            raise ResourceUnavailableError("resource is unavailable")
        deletion_exists = self._session.scalar(
            select(DeletionDirective.id).where(
                DeletionDirective.owner_id == owner_id,
                DeletionDirective.resource_record_id == model.id,
            )
        )
        if deletion_exists is not None:
            self._session.rollback()
            raise ResourceUnavailableError("resource is unavailable")
        try:
            return self._resource_view(model)
        finally:
            self._session.rollback()

    def request_deletion(
        self,
        *,
        owner_id: UUID,
        operation_id: UUID,
        resource_type: str,
        resource_id: UUID,
        reason: DeletionReason,
    ) -> DeletionView:
        self._validate_resource_type(resource_type)
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            operation_match = self._session.scalar(
                select(DeletionDirective)
                .where(
                    DeletionDirective.owner_id == owner_id,
                    DeletionDirective.operation_id == operation_id,
                )
                .with_for_update()
            )
            resource = self._resource_by_identity(owner_id, resource_type, resource_id)
            if resource is None:
                raise ResourceUnavailableError("resource is unavailable")
            if operation_match is not None:
                if operation_match.resource_record_id != resource.id:
                    raise LifecycleConflictError(
                        "operation ID is already used for another resource"
                    )
                return self._deletion_view(operation_match, resource)
            existing = self._session.scalar(
                select(DeletionDirective)
                .where(
                    DeletionDirective.owner_id == owner_id,
                    DeletionDirective.resource_record_id == resource.id,
                )
                .with_for_update()
            )
            if existing is not None:
                return self._deletion_view(existing, resource)
            model = self._create_deletion(
                resource=resource,
                operation_id=operation_id,
                reason=reason,
                now=now,
            )
            self._session.flush()
            view = self._deletion_view(model, resource)
        return view

    def expire_due(self, *, limit: int) -> list[DeletionView]:
        if not 1 <= limit <= 1000:
            raise ValueError("limit must be between 1 and 1000")
        now = self._clock()
        deletion_exists = (
            select(DeletionDirective.id)
            .where(DeletionDirective.resource_record_id == EvidenceResource.id)
            .exists()
        )
        self._session.rollback()
        with self._session.begin():
            resources = self._session.scalars(
                select(EvidenceResource)
                .where(
                    EvidenceResource.expires_at <= now,
                    ~deletion_exists,
                )
                .order_by(EvidenceResource.expires_at, EvidenceResource.id)
                .limit(limit)
                .with_for_update(skip_locked=True)
            ).all()
            models = [
                self._create_deletion(
                    resource=resource,
                    operation_id=uuid4(),
                    reason=DeletionReason.RETENTION_EXPIRED,
                    now=now,
                )
                for resource in resources
            ]
            self._session.flush()
            views = [
                self._deletion_view(model, resource)
                for model, resource in zip(models, resources, strict=True)
            ]
        return views

    def get_deletion(self, *, owner_id: UUID, deletion_id: UUID) -> DeletionView:
        model = self._session.scalar(
            select(DeletionDirective).where(
                DeletionDirective.owner_id == owner_id,
                DeletionDirective.id == deletion_id,
            )
        )
        if model is None:
            self._session.rollback()
            raise ResourceUnavailableError("deletion record is unavailable")
        resource = self._session.get(EvidenceResource, model.resource_record_id)
        if resource is None:
            self._session.rollback()
            raise ResourceUnavailableError("resource is unavailable")
        try:
            return self._deletion_view(model, resource)
        finally:
            self._session.rollback()

    def _create_deletion(
        self,
        *,
        resource: EvidenceResource,
        operation_id: UUID,
        reason: DeletionReason,
        now: datetime,
    ) -> DeletionDirective:
        target_specs = [CleanupTargetSpec.model_validate(item) for item in resource.cleanup_targets]
        is_complete = not target_specs
        model = DeletionDirective(
            id=uuid4(),
            owner_id=resource.owner_id,
            operation_id=operation_id,
            resource_record_id=resource.id,
            reason=reason.value,
            status=(DeletionStatus.COMPLETED if is_complete else DeletionStatus.PENDING).value,
            requested_at=now,
            cleanup_due_at=now + ONLINE_CLEANUP_SLA,
            completed_at=now if is_complete else None,
        )
        self._session.add(model)
        self._session.flush()
        for spec in target_specs:
            self._session.add(
                CleanupTarget(
                    id=uuid4(),
                    deletion_id=model.id,
                    target_kind=spec.kind.value,
                    target_reference=spec.reference,
                    status=CleanupStatus.PENDING.value,
                    attempt_count=0,
                    next_attempt_at=now,
                    lease_token=None,
                    lease_expires_at=None,
                    last_error_code=None,
                    completed_at=None,
                    created_at=now,
                    updated_at=now,
                )
            )
        return model

    def _resource_by_identity(
        self,
        owner_id: UUID,
        resource_type: str,
        resource_id: UUID,
    ) -> EvidenceResource | None:
        return self._session.scalar(
            select(EvidenceResource).where(
                EvidenceResource.owner_id == owner_id,
                EvidenceResource.resource_type == resource_type,
                EvidenceResource.resource_id == resource_id,
            )
        )

    def _deletion_view(
        self,
        model: DeletionDirective,
        resource: EvidenceResource,
    ) -> DeletionView:
        statuses = self._session.scalars(
            select(CleanupTarget.status).where(CleanupTarget.deletion_id == model.id)
        ).all()
        return DeletionView(
            id=model.id,
            owner_id=model.owner_id,
            operation_id=model.operation_id,
            resource_record_id=model.resource_record_id,
            resource_type=resource.resource_type,
            resource_id=resource.resource_id,
            reason=DeletionReason(model.reason),
            status=DeletionStatus(model.status),
            requested_at=model.requested_at,
            cleanup_due_at=model.cleanup_due_at,
            completed_at=model.completed_at,
            target_count=len(statuses),
            completed_targets=sum(status == CleanupStatus.SUCCEEDED.value for status in statuses),
            failed_targets=sum(status == CleanupStatus.FAILED.value for status in statuses),
        )

    @staticmethod
    def _resource_view(model: EvidenceResource) -> EvidenceResourceView:
        return EvidenceResourceView(
            id=model.id,
            owner_id=model.owner_id,
            resource_type=model.resource_type,
            resource_id=model.resource_id,
            source_policy_id=model.source_policy_id,
            source_policy_version=model.source_policy_version,
            retention_policy_id=model.retention_policy_id,
            retention_policy_version=model.retention_policy_version,
            data_class=DataClass(model.data_class),
            collected_at=model.collected_at,
            expires_at=model.expires_at,
            cleanup_targets=tuple(
                CleanupTargetSpec.model_validate(target) for target in model.cleanup_targets
            ),
            created_at=model.created_at,
        )

    @staticmethod
    def _validate_resource_type(resource_type: str) -> None:
        if re.fullmatch(r"[a-z][a-z0-9_]{0,63}", resource_type) is None:
            raise ValueError("resource_type must be a stable lowercase identifier")


CleanupHandler = Callable[[str], None]


class CleanupProcessor:
    def __init__(
        self,
        sessions: sessionmaker[Session],
        *,
        handlers: Mapping[CleanupTargetKind, CleanupHandler],
        clock: Clock | None = None,
    ) -> None:
        self._sessions = sessions
        self._handlers = handlers
        self._clock = clock or (lambda: datetime.now(UTC))

    def process_due(self, *, limit: int) -> CleanupBatchResult:
        if not 1 <= limit <= 1000:
            raise ValueError("limit must be between 1 and 1000")
        succeeded = 0
        failed = 0
        for _ in range(limit):
            lease = self._claim()
            if lease is None:
                break
            try:
                handler = self._handlers.get(lease.kind)
                if handler is None:
                    raise CleanupHandlerUnavailableError(
                        f"cleanup handler is unavailable for {lease.kind.value}"
                    )
                handler(lease.reference)
            except Exception as error:
                self._fail(lease, error_code=type(error).__name__)
                failed += 1
            else:
                self._complete(lease)
                succeeded += 1
        return CleanupBatchResult(succeeded=succeeded, failed=failed)

    def _claim(self) -> CleanupLease | None:
        now = self._clock()
        with self._sessions() as session, session.begin():
            exhausted = session.scalars(
                select(CleanupTarget)
                .where(
                    CleanupTarget.status == CleanupStatus.PROCESSING.value,
                    CleanupTarget.lease_expires_at <= now,
                    CleanupTarget.attempt_count >= MAX_CLEANUP_ATTEMPTS,
                )
                .with_for_update(skip_locked=True)
            ).all()
            for exhausted_target in exhausted:
                exhausted_target.status = CleanupStatus.FAILED.value
                exhausted_target.lease_token = None
                exhausted_target.lease_expires_at = None
                exhausted_target.next_attempt_at = None
                exhausted_target.last_error_code = "lease_expired"
                exhausted_target.updated_at = now
                session.flush()
                self._refresh_deletion(session, exhausted_target.deletion_id, now)

            target = session.scalar(
                select(CleanupTarget)
                .where(
                    or_(
                        (
                            CleanupTarget.status.in_(
                                [CleanupStatus.PENDING.value, CleanupStatus.FAILED.value]
                            )
                            & (CleanupTarget.next_attempt_at <= now)
                            & (CleanupTarget.attempt_count < MAX_CLEANUP_ATTEMPTS)
                        ),
                        (
                            (CleanupTarget.status == CleanupStatus.PROCESSING.value)
                            & (CleanupTarget.lease_expires_at <= now)
                            & (CleanupTarget.attempt_count < MAX_CLEANUP_ATTEMPTS)
                        ),
                    )
                )
                .order_by(CleanupTarget.next_attempt_at, CleanupTarget.created_at, CleanupTarget.id)
                .limit(1)
                .with_for_update(skip_locked=True)
            )
            if target is None:
                return None
            lease_token = uuid4()
            target.status = CleanupStatus.PROCESSING.value
            target.attempt_count += 1
            target.next_attempt_at = None
            target.lease_token = lease_token
            target.lease_expires_at = now + CLEANUP_LEASE_DURATION
            target.last_error_code = None
            target.updated_at = now
            return CleanupLease(
                target_id=target.id,
                deletion_id=target.deletion_id,
                kind=CleanupTargetKind(target.target_kind),
                reference=target.target_reference,
                lease_token=lease_token,
                attempt_count=target.attempt_count,
                lease_expires_at=target.lease_expires_at,
            )

    def _complete(self, lease: CleanupLease) -> None:
        now = self._clock()
        with self._sessions() as session, session.begin():
            target = self._lock_current_target(session, lease)
            target.status = CleanupStatus.SUCCEEDED.value
            target.next_attempt_at = None
            target.lease_token = None
            target.lease_expires_at = None
            target.last_error_code = None
            target.completed_at = now
            target.updated_at = now
            session.flush()
            self._refresh_deletion(session, target.deletion_id, now)

    def _fail(self, lease: CleanupLease, *, error_code: str) -> None:
        now = self._clock()
        with self._sessions() as session, session.begin():
            target = self._lock_current_target(session, lease)
            target.status = CleanupStatus.FAILED.value
            target.lease_token = None
            target.lease_expires_at = None
            target.last_error_code = error_code[:128]
            target.next_attempt_at = (
                now + retry_delay(target.attempt_count)
                if target.attempt_count < MAX_CLEANUP_ATTEMPTS
                else None
            )
            target.updated_at = now
            session.flush()
            self._refresh_deletion(session, target.deletion_id, now)

    @staticmethod
    def _lock_current_target(session: Session, lease: CleanupLease) -> CleanupTarget:
        target = session.scalar(
            select(CleanupTarget).where(CleanupTarget.id == lease.target_id).with_for_update()
        )
        if (
            target is None
            or target.status != CleanupStatus.PROCESSING.value
            or target.lease_token != lease.lease_token
        ):
            raise StaleCleanupLeaseError("cleanup lease is stale")
        return target

    @staticmethod
    def _refresh_deletion(session: Session, deletion_id: UUID, now: datetime) -> None:
        deletion = session.scalar(
            select(DeletionDirective).where(DeletionDirective.id == deletion_id).with_for_update()
        )
        if deletion is None:
            raise ResourceUnavailableError("deletion record is unavailable")
        targets = session.scalars(
            select(CleanupTarget).where(CleanupTarget.deletion_id == deletion_id)
        ).all()
        if targets and all(target.status == CleanupStatus.SUCCEEDED.value for target in targets):
            deletion.status = DeletionStatus.COMPLETED.value
            deletion.completed_at = now
        elif any(
            target.status == CleanupStatus.FAILED.value
            and target.attempt_count >= MAX_CLEANUP_ATTEMPTS
            and target.next_attempt_at is None
            for target in targets
        ):
            deletion.status = DeletionStatus.FAILED.value
            deletion.completed_at = None
        else:
            deletion.status = DeletionStatus.PENDING.value
            deletion.completed_at = None
