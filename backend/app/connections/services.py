from __future__ import annotations

import hashlib
from collections.abc import Callable, Iterable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID, uuid4

from pydantic import SecretStr
from sqlalchemy import and_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session

from connections.catalog import CAPABILITY_LABELS, SOURCE_CATALOG, SourceCatalogEntry
from connections.models import (
    SourceCapabilityEvidence,
    SourceConnection,
    SourceConnectionVersion,
)
from connections.schemas import (
    ConnectionEvidenceKind,
    ConnectionEvidenceOutcome,
    PersistedReadEvidenceInput,
    ProbeEvidenceInput,
    SourceCapabilityEvidenceView,
    SourceCapabilityStatus,
    SourceCapabilityView,
    SourceConnectionAuthKind,
    SourceConnectionStatus,
    SourceConnectionUpdateInput,
    SourceConnectionView,
    SourceEntryPoint,
    SourceEntryPointView,
    SourcePlatformStatus,
    SourcePlatformView,
    SourceRolloutRole,
)
from core.errors import ApplicationError
from evidence.services import load_source_access_readiness
from sources.contracts import SourceCapability, SourceStopReason

type Clock = Callable[[], datetime]


@dataclass(frozen=True, slots=True)
class CapabilityStatusFacts:
    product_restricted: bool = False
    connection_status: SourceConnectionStatus | None = None
    policy_ready: bool = False
    evidence_kind: ConnectionEvidenceKind | None = None
    evidence_outcome: ConnectionEvidenceOutcome | None = None
    stop_reason: SourceStopReason | None = None
    last_checked_at: datetime | None = None
    last_persisted_success_at: datetime | None = None


def resolve_capability_status(facts: CapabilityStatusFacts) -> SourceCapabilityStatus:
    if facts.product_restricted:
        return SourceCapabilityStatus.RESTRICTED
    if facts.connection_status is None:
        return SourceCapabilityStatus.UNCONFIGURED
    if facts.connection_status is SourceConnectionStatus.DISABLED:
        return SourceCapabilityStatus.DISABLED
    if not facts.policy_ready:
        return SourceCapabilityStatus.RESTRICTED
    if facts.evidence_outcome is ConnectionEvidenceOutcome.FAILED:
        if facts.stop_reason is SourceStopReason.AUTHENTICATION_REQUIRED:
            return SourceCapabilityStatus.AUTHENTICATION_REQUIRED
        if facts.stop_reason in {
            SourceStopReason.ACCESS_DENIED,
            SourceStopReason.UNSUPPORTED,
            SourceStopReason.BUDGET_EXHAUSTED,
        }:
            return SourceCapabilityStatus.RESTRICTED
        return SourceCapabilityStatus.PENDING_VERIFICATION
    if (
        facts.evidence_kind is ConnectionEvidenceKind.PERSISTED_READ
        and facts.evidence_outcome is ConnectionEvidenceOutcome.SUCCEEDED
    ):
        return SourceCapabilityStatus.AVAILABLE
    return SourceCapabilityStatus.PENDING_VERIFICATION


def aggregate_platform_status(
    statuses: Iterable[SourceCapabilityStatus],
) -> SourcePlatformStatus:
    values = tuple(statuses)
    if not values:
        return SourcePlatformStatus.UNCONFIGURED
    unique = set(values)
    if len(unique) == 1:
        return SourcePlatformStatus(next(iter(unique)).value)
    if SourceCapabilityStatus.AVAILABLE in unique:
        return SourcePlatformStatus.PARTIAL
    priority = (
        SourceCapabilityStatus.RESTRICTED,
        SourceCapabilityStatus.AUTHENTICATION_REQUIRED,
        SourceCapabilityStatus.DISABLED,
        SourceCapabilityStatus.PENDING_VERIFICATION,
        SourceCapabilityStatus.UNCONFIGURED,
    )
    return SourcePlatformStatus(next(status.value for status in priority if status in unique))


class SourceCapabilityEvidenceService:
    def __init__(self, session: Session, *, clock: Clock | None = None) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def record_probe(
        self,
        *,
        owner_id: UUID,
        command: ProbeEvidenceInput,
    ) -> SourceCapabilityEvidenceView:
        return self._record(
            owner_id=owner_id,
            command=command,
            kind=ConnectionEvidenceKind.PROBE,
            resource_ref=None,
        )

    def record_persisted_read(
        self,
        *,
        owner_id: UUID,
        command: PersistedReadEvidenceInput,
    ) -> SourceCapabilityEvidenceView:
        return self._record(
            owner_id=owner_id,
            command=command,
            kind=ConnectionEvidenceKind.PERSISTED_READ,
            resource_ref=command.resource_ref,
        )

    def record_persisted_read_in_transaction(
        self,
        *,
        owner_id: UUID,
        command: PersistedReadEvidenceInput,
    ) -> SourceCapabilityEvidenceView:
        """Record persisted-read evidence inside an existing outer transaction."""
        return self._record_in_transaction(
            owner_id=owner_id,
            command=command,
            kind=ConnectionEvidenceKind.PERSISTED_READ,
            resource_ref=command.resource_ref,
        )

    def _record(
        self,
        *,
        owner_id: UUID,
        command: ProbeEvidenceInput | PersistedReadEvidenceInput,
        kind: ConnectionEvidenceKind,
        resource_ref: str | None,
    ) -> SourceCapabilityEvidenceView:
        self._session.rollback()
        with self._session.begin():
            return self._record_in_transaction(
                owner_id=owner_id,
                command=command,
                kind=kind,
                resource_ref=resource_ref,
            )

    def _record_in_transaction(
        self,
        *,
        owner_id: UUID,
        command: ProbeEvidenceInput | PersistedReadEvidenceInput,
        kind: ConnectionEvidenceKind,
        resource_ref: str | None,
    ) -> SourceCapabilityEvidenceView:
        now = self._clock()
        if now.tzinfo is None:
            raise ValueError("clock must return a timezone-aware datetime")
        stop_reason = command.stop_reason.value if command.stop_reason is not None else None
        connection = self._session.scalar(
            select(SourceConnection)
            .where(
                SourceConnection.owner_id == owner_id,
                SourceConnection.id == command.connection_id,
            )
            .with_for_update()
        )
        if connection is None:
            raise ApplicationError("resource_not_found")

        existing = self._session.scalar(
            select(SourceCapabilityEvidence).where(
                SourceCapabilityEvidence.owner_id == owner_id,
                SourceCapabilityEvidence.operation_id == command.operation_id,
            )
        )
        if existing is not None:
            if not self._matches(
                existing,
                command=command,
                kind=kind,
                stop_reason=stop_reason,
                resource_ref=resource_ref,
            ):
                raise ApplicationError("idempotency_conflict")
            return self._view(existing)
        if connection.status == SourceConnectionStatus.DISABLED.value:
            raise ApplicationError("connection_disabled")
        if command.connection_version != connection.current_version:
            raise ApplicationError("connection_version_conflict")

        evidence_id = uuid4()
        inserted_id = self._session.scalar(
            insert(SourceCapabilityEvidence)
            .values(
                id=evidence_id,
                operation_id=command.operation_id,
                owner_id=owner_id,
                connection_id=connection.id,
                connection_version=command.connection_version,
                capability=command.capability.value,
                entry_point=command.entry_point.value,
                kind=kind.value,
                outcome=command.outcome.value,
                stop_reason=stop_reason,
                resource_ref=resource_ref,
                component_name=command.component_name,
                component_version=command.component_version,
                observed_at=now,
                created_at=now,
            )
            .on_conflict_do_nothing(constraint="source_capability_evidence_owner_operation_key")
            .returning(SourceCapabilityEvidence.id)
        )
        evidence: SourceCapabilityEvidence
        if inserted_id is not None:
            evidence = SourceCapabilityEvidence(
                id=inserted_id,
                operation_id=command.operation_id,
                owner_id=owner_id,
                connection_id=connection.id,
                connection_version=command.connection_version,
                capability=command.capability.value,
                entry_point=command.entry_point.value,
                kind=kind.value,
                outcome=command.outcome.value,
                stop_reason=stop_reason,
                resource_ref=resource_ref,
                component_name=command.component_name,
                component_version=command.component_version,
                observed_at=now,
                created_at=now,
            )
        else:
            existing = self._session.scalar(
                select(SourceCapabilityEvidence).where(
                    SourceCapabilityEvidence.owner_id == owner_id,
                    SourceCapabilityEvidence.operation_id == command.operation_id,
                )
            )
            if existing is None:
                raise RuntimeError("conflicting capability evidence is not visible")
            if not self._matches(
                existing,
                command=command,
                kind=kind,
                stop_reason=stop_reason,
                resource_ref=resource_ref,
            ):
                raise ApplicationError("idempotency_conflict")
            evidence = existing
        return self._view(evidence)

    @staticmethod
    def _matches(
        evidence: SourceCapabilityEvidence,
        *,
        command: ProbeEvidenceInput | PersistedReadEvidenceInput,
        kind: ConnectionEvidenceKind,
        stop_reason: str | None,
        resource_ref: str | None,
    ) -> bool:
        return (
            evidence.connection_id == command.connection_id
            and evidence.connection_version == command.connection_version
            and evidence.capability == command.capability.value
            and evidence.entry_point == command.entry_point.value
            and evidence.kind == kind.value
            and evidence.outcome == command.outcome.value
            and evidence.stop_reason == stop_reason
            and evidence.resource_ref == resource_ref
            and evidence.component_name == command.component_name
            and evidence.component_version == command.component_version
        )

    @staticmethod
    def _view(evidence: SourceCapabilityEvidence) -> SourceCapabilityEvidenceView:
        return SourceCapabilityEvidenceView.model_validate(evidence)


def _next_action(
    status: SourceCapabilityStatus,
    catalog: SourceCatalogEntry,
) -> str:
    if catalog.product_restricted:
        return catalog.restricted_next_action
    return {
        SourceCapabilityStatus.UNCONFIGURED: "确认平台范围与授权条件后再配置连接。",
        SourceCapabilityStatus.PENDING_VERIFICATION: "完成对应入口的持久读取验证。",
        SourceCapabilityStatus.AVAILABLE: "已验证。启用任务仍需经过预算与任务门禁。",
        SourceCapabilityStatus.AUTHENTICATION_REQUIRED: "重新授权并使用新连接版本验证。",
        SourceCapabilityStatus.RESTRICTED: catalog.restricted_next_action,
        SourceCapabilityStatus.DISABLED: "连接已停用。重新启用后必须再次验证。",
    }[status]


class SourceConnectionService:
    def __init__(
        self,
        session: Session,
        *,
        credentials: Mapping[str, SecretStr] | None = None,
        clock: Clock | None = None,
    ) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))
        self._credential_refs = {
            key: source_credential_reference(key, value)
            for key, value in (credentials or {}).items()
        }

    def update_connection(
        self, *, owner_id: UUID, source_key: str, command: SourceConnectionUpdateInput
    ) -> SourceConnectionView:
        catalog = next((item for item in SOURCE_CATALOG if item.source_key == source_key), None)
        if catalog is None:
            raise ApplicationError("resource_not_found")
        secret_ref = (
            self._credential_refs.get(source_key)
            if catalog.auth_kind is SourceConnectionAuthKind.SERVER_CREDENTIAL
            else None
        )
        if (
            command.status is SourceConnectionStatus.ACTIVE
            and catalog.auth_kind is SourceConnectionAuthKind.SERVER_CREDENTIAL
            and secret_ref is None
        ):
            raise ApplicationError("connection_credentials_missing")
        now = self._clock()
        if now.tzinfo is None:
            raise ValueError("clock must return a timezone-aware datetime")
        now = now.astimezone(UTC)
        self._session.rollback()
        with self._session.begin():
            if command.expected_version == 0 and command.status is SourceConnectionStatus.ACTIVE:
                connection_id = self._session.scalar(
                    insert(SourceConnection)
                    .values(
                        id=uuid4(),
                        owner_id=owner_id,
                        source_key=source_key,
                        status=command.status.value,
                        current_version=1,
                        created_at=now,
                        updated_at=now,
                    )
                    .on_conflict_do_nothing(constraint="source_connections_owner_source_key")
                    .returning(SourceConnection.id)
                )
                if connection_id is not None:
                    self._session.add(
                        SourceConnectionVersion(
                            connection_id=connection_id,
                            owner_id=owner_id,
                            version=1,
                            auth_kind=catalog.auth_kind.value,
                            secret_ref=secret_ref,
                            created_by=owner_id,
                            created_at=now,
                        )
                    )
                    return SourceConnectionView(
                        id=connection_id,
                        source_key=source_key,
                        status=command.status,
                        version=1,
                        updated_at=now,
                    )
            connection = self._session.scalar(
                select(SourceConnection)
                .where(
                    SourceConnection.owner_id == owner_id,
                    SourceConnection.source_key == source_key,
                )
                .with_for_update()
            )
            if connection is None:
                raise ApplicationError("resource_not_found")
            previous = self._session.get(
                SourceConnectionVersion, (connection.id, connection.current_version)
            )
            if previous is None:
                raise RuntimeError("current connection version is not visible")
            if command.status is SourceConnectionStatus.DISABLED:
                target_auth_kind = SourceConnectionAuthKind(previous.auth_kind)
                target_ref = previous.secret_ref
            else:
                target_auth_kind = catalog.auth_kind
                target_ref = secret_ref
            unchanged = (
                connection.status == command.status.value
                and previous.auth_kind == target_auth_kind.value
                and previous.secret_ref == target_ref
            )
            if connection.current_version != command.expected_version:
                if not (unchanged and connection.current_version == command.expected_version + 1):
                    raise ApplicationError("connection_version_conflict")
            elif not unchanged:
                connection.current_version += 1
                connection.status = command.status.value
                connection.updated_at = now
                self._session.add(
                    SourceConnectionVersion(
                        connection_id=connection.id,
                        owner_id=owner_id,
                        version=connection.current_version,
                        auth_kind=target_auth_kind.value,
                        secret_ref=target_ref,
                        created_by=owner_id,
                        created_at=now,
                    )
                )
            return SourceConnectionView(
                id=connection.id,
                source_key=source_key,
                status=SourceConnectionStatus(connection.status),
                version=connection.current_version,
                updated_at=connection.updated_at.astimezone(UTC),
            )

    def list_platforms(self, *, owner_id: UUID) -> list[SourcePlatformView]:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            connection_rows = list(
                self._session.execute(
                    select(SourceConnection, SourceConnectionVersion)
                    .join(
                        SourceConnectionVersion,
                        and_(
                            SourceConnectionVersion.connection_id == SourceConnection.id,
                            SourceConnectionVersion.owner_id == SourceConnection.owner_id,
                            SourceConnectionVersion.version == SourceConnection.current_version,
                        ),
                    )
                    .where(SourceConnection.owner_id == owner_id)
                ).all()
            )
            connections = {connection.source_key: connection for connection, _ in connection_rows}
            current_versions = {connection.id: version for connection, version in connection_rows}
            evidence_rows = list(
                self._session.scalars(
                    select(SourceCapabilityEvidence)
                    .join(
                        SourceConnection,
                        and_(
                            SourceConnection.id == SourceCapabilityEvidence.connection_id,
                            SourceConnection.owner_id == SourceCapabilityEvidence.owner_id,
                            SourceConnection.current_version
                            == SourceCapabilityEvidence.connection_version,
                        ),
                    )
                    .where(SourceCapabilityEvidence.owner_id == owner_id)
                    .order_by(
                        SourceCapabilityEvidence.observed_at,
                        SourceCapabilityEvidence.created_at,
                        SourceCapabilityEvidence.id,
                    )
                )
            )
            policy_readiness = load_source_access_readiness(
                self._session,
                owner_id=owner_id,
                now=now,
            )

            latest: dict[tuple[UUID, str, str], SourceCapabilityEvidence] = {}
            last_success: dict[tuple[UUID, str, str], datetime] = {}
            authentication_failures: set[UUID] = set()
            for evidence in evidence_rows:
                key = (evidence.connection_id, evidence.capability, evidence.entry_point)
                latest[key] = evidence
                if evidence.stop_reason == SourceStopReason.AUTHENTICATION_REQUIRED.value:
                    authentication_failures.add(evidence.connection_id)
                if (
                    evidence.kind == ConnectionEvidenceKind.PERSISTED_READ.value
                    and evidence.outcome == ConnectionEvidenceOutcome.SUCCEEDED.value
                ):
                    last_success[key] = evidence.observed_at

            return [
                self._platform_view(
                    catalog=catalog,
                    connection=connections.get(catalog.source_key),
                    current_versions=current_versions,
                    authentication_failures=authentication_failures,
                    policy_readiness=policy_readiness,
                    latest=latest,
                    last_success=last_success,
                )
                for catalog in SOURCE_CATALOG
            ]

    def _platform_view(
        self,
        *,
        catalog: SourceCatalogEntry,
        connection: SourceConnection | None,
        current_versions: Mapping[UUID, SourceConnectionVersion],
        authentication_failures: set[UUID],
        policy_readiness: dict[tuple[str, SourceCapability], bool],
        latest: dict[tuple[UUID, str, str], SourceCapabilityEvidence],
        last_success: dict[tuple[UUID, str, str], datetime],
    ) -> SourcePlatformView:
        configured_ref = self._credential_refs.get(catalog.source_key)
        current_version = current_versions.get(connection.id) if connection is not None else None
        credential_matches = current_version is not None and (
            current_version.auth_kind == SourceConnectionAuthKind.NONE.value
            or configured_ref == current_version.secret_ref
        )
        authentication_failed = connection is not None and connection.id in authentication_failures
        capability_views: list[SourceCapabilityView] = []
        statuses: list[SourceCapabilityStatus] = []
        for capability in catalog.capabilities:
            entry_views: dict[SourceEntryPoint, SourceEntryPointView] = {}
            for entry_point in SourceEntryPoint:
                key = (
                    connection.id if connection is not None else UUID(int=0),
                    capability.value,
                    entry_point.value,
                )
                evidence = latest.get(key)
                facts = CapabilityStatusFacts(
                    product_restricted=catalog.product_restricted,
                    connection_status=(
                        SourceConnectionStatus(connection.status)
                        if connection is not None
                        else None
                    ),
                    policy_ready=policy_readiness.get(
                        (catalog.source_key, capability),
                        False,
                    ),
                    evidence_kind=(
                        ConnectionEvidenceKind(evidence.kind) if evidence is not None else None
                    ),
                    evidence_outcome=(
                        ConnectionEvidenceOutcome(evidence.outcome)
                        if evidence is not None
                        else None
                    ),
                    stop_reason=(
                        SourceStopReason(evidence.stop_reason)
                        if evidence is not None and evidence.stop_reason is not None
                        else None
                    ),
                    last_checked_at=evidence.observed_at if evidence is not None else None,
                    last_persisted_success_at=last_success.get(key),
                )
                status = resolve_capability_status(facts)
                if (
                    connection is not None
                    and connection.status == SourceConnectionStatus.ACTIVE.value
                    and not catalog.product_restricted
                    and (not credential_matches or authentication_failed)
                ):
                    status = SourceCapabilityStatus.AUTHENTICATION_REQUIRED
                    facts = CapabilityStatusFacts(
                        stop_reason=SourceStopReason.AUTHENTICATION_REQUIRED,
                        last_checked_at=facts.last_checked_at,
                        last_persisted_success_at=facts.last_persisted_success_at,
                    )
                statuses.append(status)
                entry_views[entry_point] = SourceEntryPointView(
                    status=status,
                    last_checked_at=facts.last_checked_at,
                    last_persisted_success_at=facts.last_persisted_success_at,
                    stop_reason=facts.stop_reason,
                    next_action=_next_action(status, catalog),
                )
            capability_views.append(
                SourceCapabilityView(
                    capability=capability,
                    display_name=CAPABILITY_LABELS[capability],
                    manual=entry_views[SourceEntryPoint.MANUAL],
                    scheduled=entry_views[SourceEntryPoint.SCHEDULED],
                )
            )

        return SourcePlatformView(
            source_key=catalog.source_key,
            display_name=catalog.display_name,
            rollout_role=SourceRolloutRole(catalog.rollout_role),
            status=aggregate_platform_status(statuses),
            connection_version=connection.current_version if connection is not None else None,
            has_credentials=current_version is not None and current_version.secret_ref is not None,
            connection_id=connection.id if connection is not None else None,
            connection_status=SourceConnectionStatus(connection.status) if connection else None,
            credential_configured=configured_ref is not None,
            credential_update_available=connection is not None
            and catalog.auth_kind is SourceConnectionAuthKind.SERVER_CREDENTIAL
            and configured_ref is not None
            and not credential_matches,
            capabilities=capability_views,
        )


def source_credential_reference(source_key: str, credential: SecretStr) -> str:
    """Return a non-reversible server-only reference; never expose it over HTTP."""
    digest = hashlib.sha256(credential.get_secret_value().encode()).hexdigest()
    return f"settings:{source_key}:{digest}"


def require_source_connection_enabled(
    session: Session, *, owner_id: UUID, source_key: str | None
) -> None:
    """Apply an existing connection's stop barrier in the caller's transaction."""
    if source_key is None:
        return
    connection = session.scalar(
        select(SourceConnection)
        .where(SourceConnection.owner_id == owner_id, SourceConnection.source_key == source_key)
        .with_for_update()
    )
    if connection is not None and connection.status == SourceConnectionStatus.DISABLED.value:
        raise ApplicationError("connection_disabled")
    if (
        connection is not None
        and session.scalar(
            select(SourceCapabilityEvidence.id)
            .where(
                SourceCapabilityEvidence.owner_id == owner_id,
                SourceCapabilityEvidence.connection_id == connection.id,
                SourceCapabilityEvidence.connection_version == connection.current_version,
                SourceCapabilityEvidence.stop_reason
                == SourceStopReason.AUTHENTICATION_REQUIRED.value,
            )
            .limit(1)
        )
        is not None
    ):
        raise ApplicationError("connection_authentication_required")
