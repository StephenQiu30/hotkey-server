from __future__ import annotations

import hashlib
from collections.abc import Callable, Iterable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID, uuid4

from playwright.async_api import StorageState
from pydantic import SecretStr
from sqlalchemy import and_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session

from connections.adapters.local_secrets import BrowserStateError, BrowserStateStore
from connections.catalog import CAPABILITY_LABELS, SOURCE_CATALOG, SourceCatalogEntry
from connections.models import (
    SourceCapabilityEvidence,
    SourceConnection,
    SourceConnectionVersion,
)
from connections.presets import SOURCE_PRESETS, SourcePreset
from connections.schemas import (
    ConnectionEvidenceKind,
    ConnectionEvidenceOutcome,
    PersistedReadEvidenceInput,
    ProbeEvidenceInput,
    SourceCapabilityEvidenceView,
    SourceCapabilityStatus,
    SourceCapabilityView,
    SourceConnectionAuthKind,
    SourceConnectionConfig,
    SourceConnectionStatus,
    SourceConnectionUpdateInput,
    SourceConnectionView,
    SourceEntryPoint,
    SourceEntryPointView,
    SourcePlatformStatus,
    SourcePlatformView,
    SourcePresetApplyView,
    SourceRolloutRole,
)
from core.errors import ApplicationError
from evidence.schemas import (
    AccessBasis,
    AccessPolicyStatus,
    DataClass,
    RetentionPolicyInput,
    SourceAccessPolicyInput,
)
from evidence.services import (
    RetentionPolicyService,
    SourceAccessPolicyService,
    load_source_access_readiness,
)
from sources.adapters.web_targets import normalize_web_url
from sources.contracts import SourceCapability, SourceStopReason

type Clock = Callable[[], datetime]


@dataclass(frozen=True, slots=True)
class AppliedSourcePreset:
    source_key: str
    connection_id: UUID
    connection_version: int
    capabilities: tuple[SourceCapability, ...]


@dataclass(frozen=True, slots=True)
class AppliedHotlistPreset:
    owner_id: UUID
    source_key: str
    connection_id: UUID
    connection_version: int


def list_applied_hotlist_presets_in_transaction(
    session: Session, *, owner_id: UUID | None = None
) -> tuple[AppliedHotlistPreset, ...]:
    if not session.in_transaction():
        raise RuntimeError("applied hotlist scan requires the caller's transaction")
    keys = tuple(
        key
        for key, preset in SOURCE_PRESETS.items()
        if any(item.capability is SourceCapability.HOTLIST for item in preset.capabilities)
    )
    statement = (
        select(SourceConnection.owner_id)
        .where(
            SourceConnection.source_key.in_(keys),
            SourceConnection.status == SourceConnectionStatus.ACTIVE.value,
        )
        .distinct()
    )
    if owner_id is not None:
        statement = statement.where(SourceConnection.owner_id == owner_id)
    result: list[AppliedHotlistPreset] = []
    for current_owner in tuple(session.scalars(statement)):
        applied = load_applied_source_presets_in_transaction(
            session, owner_id=current_owner, source_keys=keys
        )
        result.extend(
            AppliedHotlistPreset(
                owner_id=current_owner,
                source_key=key,
                connection_id=preset.connection_id,
                connection_version=preset.connection_version,
            )
            for key, preset in applied.items()
            if SourceCapability.HOTLIST in preset.capabilities
        )
    return tuple(sorted(result, key=lambda item: (item.owner_id, item.source_key)))


def load_applied_source_presets_in_transaction(
    session: Session,
    *,
    owner_id: UUID,
    source_keys: Iterable[str],
) -> dict[str, AppliedSourcePreset]:
    """Return requested presets whose current active connection still matches the preset."""
    if not session.in_transaction():
        raise RuntimeError("applied source presets require the caller's transaction")
    requested = tuple(dict.fromkeys(source_keys))
    if not requested:
        return {}
    rows = session.execute(
        select(SourceConnection, SourceConnectionVersion)
        .join(
            SourceConnectionVersion,
            and_(
                SourceConnectionVersion.connection_id == SourceConnection.id,
                SourceConnectionVersion.owner_id == SourceConnection.owner_id,
                SourceConnectionVersion.version == SourceConnection.current_version,
            ),
        )
        .where(
            SourceConnection.owner_id == owner_id,
            SourceConnection.source_key.in_(requested),
            SourceConnection.status == SourceConnectionStatus.ACTIVE.value,
        )
        .with_for_update(of=SourceConnection)
    ).all()
    applied: dict[str, AppliedSourcePreset] = {}
    for connection, version in rows:
        preset = SOURCE_PRESETS.get(connection.source_key)
        if preset is None:
            continue
        expected_config = SourceConnectionConfig.model_validate(dict(preset.config)).model_dump(
            mode="json",
            exclude_defaults=True,
            exclude_none=True,
        )
        if (
            version.auth_kind != SourceConnectionAuthKind.NONE.value
            or version.secret_ref is not None
            or version.config != expected_config
        ):
            continue
        applied[connection.source_key] = AppliedSourcePreset(
            source_key=connection.source_key,
            connection_id=connection.id,
            connection_version=connection.current_version,
            capabilities=tuple(item.capability for item in preset.capabilities),
        )
    return applied


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

    def list_failed_persisted_read_operation_ids(
        self,
        *,
        owner_id: UUID,
        operation_ids: Iterable[UUID],
    ) -> frozenset[UUID]:
        """Return only IDs backed by durable failed-read evidence for this owner."""
        requested_ids = tuple(set(operation_ids))
        if not requested_ids:
            return frozenset()
        return frozenset(
            self._session.scalars(
                select(SourceCapabilityEvidence.operation_id).where(
                    SourceCapabilityEvidence.owner_id == owner_id,
                    SourceCapabilityEvidence.operation_id.in_(requested_ids),
                    SourceCapabilityEvidence.kind == ConnectionEvidenceKind.PERSISTED_READ.value,
                    SourceCapabilityEvidence.capability == SourceCapability.PAGE_CONTENT.value,
                    SourceCapabilityEvidence.outcome == ConnectionEvidenceOutcome.FAILED.value,
                    SourceCapabilityEvidence.component_name == "firecrawl",
                )
            )
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


class SourcePresetService:
    def __init__(
        self,
        session: Session,
        *,
        clock: Clock | None = None,
    ) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def apply_in_transaction(
        self,
        *,
        owner_id: UUID,
        preset: SourcePreset,
    ) -> SourcePresetApplyView:
        """Apply one complete source preset inside the caller's transaction."""
        catalog = next(
            (item for item in SOURCE_CATALOG if item.source_key == preset.source_key), None
        )
        if catalog is None or catalog.auth_kind is not SourceConnectionAuthKind.NONE:
            raise ValueError("source preset is not registered as a credential-free source")
        preset_capabilities = tuple(item.capability for item in preset.capabilities)
        if not preset_capabilities or set(preset_capabilities) != set(catalog.capabilities):
            raise ValueError("source preset capabilities do not match the source catalog")

        now = self._clock()
        if now.tzinfo is None:
            raise ValueError("clock must return a timezone-aware datetime")
        now = now.astimezone(UTC)
        config = SourceConnectionConfig.model_validate(dict(preset.config)).model_dump(
            mode="json",
            exclude_defaults=True,
            exclude_none=True,
        )
        if not config.get("allowed_hosts"):
            raise ValueError("source preset requires at least one allowed host")

        preset_changed = False
        access_service = SourceAccessPolicyService(self._session, clock=self._clock)
        retention_service = RetentionPolicyService(self._session, clock=self._clock)
        for capability in preset.capabilities:
            access_command = SourceAccessPolicyInput(
                source_key=preset.source_key,
                capability=capability.capability,
                status=AccessPolicyStatus.APPROVED,
                enabled=True,
                access_basis=AccessBasis.PUBLIC_WEB,
                terms_reference=preset.access_terms_reference,
                processing_purpose=capability.processing_purpose,
                component_name=preset.component_name,
                component_version=preset.component_version,
                component_license=preset.component_license,
                field_purposes=dict(capability.field_purposes),
                reviewed_at=preset.reviewed_at,
            )
            preset_changed |= not access_service.matches_in_transaction(
                owner_id=owner_id, command=access_command
            )
            access = access_service.save_in_transaction(owner_id=owner_id, command=access_command)
            retention_command = RetentionPolicyInput(
                source_policy_id=access.id,
                data_class=DataClass.STRUCTURED,
                requested_days=preset.retention_days,
                source_max_days=preset.retention_days,
            )
            preset_changed |= not retention_service.matches_in_transaction(
                owner_id=owner_id, command=retention_command
            )
            retention_service.save_in_transaction(owner_id=owner_id, command=retention_command)

        from jobs.schemas import (
            BudgetMetric,
            BudgetPolicyInput,
            BudgetScopeKind,
            ComponentPolicyInput,
            CostClass,
        )
        from jobs.services import ResourceBudgetService

        component_command = ComponentPolicyInput(
            component_key=preset.component_name,
            component_version=preset.component_version,
            cost_class=CostClass(preset.component_cost_class),
            enabled_for_core=True,
            terms_reference=preset.component_terms_reference,
            reviewed_at=preset.reviewed_at,
        )
        budget_command = BudgetPolicyInput(
            budget_key=preset.budget.budget_key,
            metric=BudgetMetric(preset.budget.metric),
            scope_kind=BudgetScopeKind(preset.budget.scope_kind),
            scope_reference=preset.budget.scope_reference,
            limit_units=preset.budget.limit_units,
            window_seconds=preset.budget.window_seconds,
            window_anchor_at=preset.budget.window_anchor_at,
            enabled=True,
        )
        resources = ResourceBudgetService(self._session, clock=self._clock)
        preset_changed |= not resources.component_policy_matches_in_transaction(
            owner_id=owner_id, command=component_command
        )
        resources.save_component_policy_in_transaction(owner_id=owner_id, command=component_command)
        preset_changed |= not resources.budget_policy_matches_in_transaction(
            owner_id=owner_id, command=budget_command
        )
        resources.save_budget_policy_in_transaction(owner_id=owner_id, command=budget_command)

        connection = self._session.scalar(
            select(SourceConnection)
            .where(
                SourceConnection.owner_id == owner_id,
                SourceConnection.source_key == preset.source_key,
            )
            .with_for_update()
        )
        if connection is None:
            connection = SourceConnection(
                id=uuid4(),
                owner_id=owner_id,
                source_key=preset.source_key,
                status=SourceConnectionStatus.ACTIVE.value,
                current_version=1,
                created_at=now,
                updated_at=now,
            )
            self._session.add(connection)
            self._session.add(
                SourceConnectionVersion(
                    connection_id=connection.id,
                    owner_id=owner_id,
                    version=1,
                    auth_kind=SourceConnectionAuthKind.NONE.value,
                    secret_ref=None,
                    config=config,
                    created_by=owner_id,
                    created_at=now,
                )
            )
        else:
            current = self._session.get(
                SourceConnectionVersion, (connection.id, connection.current_version)
            )
            if current is None:
                raise RuntimeError("current connection version is not visible")
            connection_changed = (
                connection.status != SourceConnectionStatus.ACTIVE.value
                or current.auth_kind != SourceConnectionAuthKind.NONE.value
                or current.secret_ref is not None
                or current.config != config
            )
            if preset_changed or connection_changed:
                connection.current_version += 1
                connection.status = SourceConnectionStatus.ACTIVE.value
                connection.updated_at = now
                self._session.add(
                    SourceConnectionVersion(
                        connection_id=connection.id,
                        owner_id=owner_id,
                        version=connection.current_version,
                        auth_kind=SourceConnectionAuthKind.NONE.value,
                        secret_ref=None,
                        config=config,
                        created_by=owner_id,
                        created_at=now,
                    )
                )
        self._session.flush()
        return SourcePresetApplyView(
            source_key=preset.source_key,
            connection_id=connection.id,
            connection_version=connection.current_version,
            capabilities=preset_capabilities,
        )


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
        if (
            catalog.auth_kind is SourceConnectionAuthKind.SERVER_CREDENTIAL
            and command.allowed_hosts
        ):
            raise ApplicationError("invalid_connection_configuration")
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
                    if catalog.auth_kind is SourceConnectionAuthKind.NONE:
                        if not command.allowed_hosts:
                            raise ApplicationError("invalid_connection_configuration")
                        config = {"allowed_hosts": list(command.allowed_hosts)}
                    else:
                        config = {}
                    self._session.add(
                        SourceConnectionVersion(
                            connection_id=connection_id,
                            owner_id=owner_id,
                            version=1,
                            auth_kind=catalog.auth_kind.value,
                            secret_ref=secret_ref,
                            config=config,
                            created_by=owner_id,
                            created_at=now,
                        )
                    )
                    return SourceConnectionView(
                        id=connection_id,
                        source_key=source_key,
                        status=command.status,
                        version=1,
                        allowed_hosts=list(command.allowed_hosts),
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
                target_config = dict(previous.config)
            else:
                target_auth_kind = catalog.auth_kind
                target_ref = secret_ref
                if target_auth_kind is SourceConnectionAuthKind.NONE:
                    target_config = (
                        {"allowed_hosts": list(command.allowed_hosts)}
                        if command.allowed_hosts
                        else dict(previous.config)
                    )
                    if not self._allowed_hosts(target_config):
                        raise ApplicationError("invalid_connection_configuration")
                else:
                    target_config = {}
            unchanged = (
                connection.status == command.status.value
                and previous.auth_kind == target_auth_kind.value
                and previous.secret_ref == target_ref
                and previous.config == target_config
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
                        config=target_config,
                        created_by=owner_id,
                        created_at=now,
                    )
                )
            return SourceConnectionView(
                id=connection.id,
                source_key=source_key,
                status=SourceConnectionStatus(connection.status),
                version=connection.current_version,
                allowed_hosts=list(self._allowed_hosts(target_config)),
                updated_at=connection.updated_at.astimezone(UTC),
            )

    def rotate_browser_state(
        self,
        *,
        owner_id: UUID,
        connection_id: UUID,
        expected_version: int,
        state: StorageState,
        store: BrowserStateStore,
    ) -> SourceConnectionView:
        """Activate one new immutable browser-state version from an operator capture."""
        if not state.get("cookies") and not state.get("origins"):
            raise ApplicationError("connection_credentials_missing")
        now = self._browser_maintenance_time()
        self._session.rollback()
        with self._session.begin():
            connection, previous = self._lock_browser_version(
                owner_id, connection_id, expected_version
            )
            new_version = connection.current_version + 1
            reference = store.reference(owner_id, connection_id, new_version)
            try:
                store.save(
                    owner_id=owner_id,
                    connection_id=connection_id,
                    version=new_version,
                    state=state,
                )
            except BrowserStateError as error:
                if str(error) != "browser_state_already_exists":
                    raise ApplicationError("connection_credentials_missing") from error
                try:
                    existing = store.load(
                        owner_id=owner_id,
                        connection_id=connection_id,
                        version=new_version,
                        reference=reference,
                    )
                except BrowserStateError as read_error:
                    raise ApplicationError("connection_credentials_missing") from read_error
                if existing != state:
                    raise ApplicationError("idempotency_conflict") from error
            connection.current_version = new_version
            connection.status = SourceConnectionStatus.ACTIVE.value
            connection.updated_at = now
            self._session.add(
                SourceConnectionVersion(
                    connection_id=connection_id,
                    owner_id=owner_id,
                    version=new_version,
                    auth_kind=SourceConnectionAuthKind.BROWSER_STATE.value,
                    secret_ref=reference,
                    config=dict(previous.config),
                    created_by=owner_id,
                    created_at=now,
                )
            )
            return self._browser_view(connection, previous)

    def disable_browser_state(
        self,
        *,
        owner_id: UUID,
        connection_id: UUID,
        expected_version: int,
    ) -> SourceConnectionView:
        """Stop browser execution; reactivation requires another operator capture."""
        now = self._browser_maintenance_time()
        self._session.rollback()
        with self._session.begin():
            connection, previous = self._lock_browser_version(
                owner_id, connection_id, expected_version
            )
            if connection.status == SourceConnectionStatus.DISABLED.value:
                return self._browser_view(connection, previous)
            new_version = connection.current_version + 1
            connection.current_version = new_version
            connection.status = SourceConnectionStatus.DISABLED.value
            connection.updated_at = now
            self._session.add(
                SourceConnectionVersion(
                    connection_id=connection_id,
                    owner_id=owner_id,
                    version=new_version,
                    auth_kind=SourceConnectionAuthKind.BROWSER_STATE.value,
                    secret_ref=BrowserStateStore.reference(owner_id, connection_id, new_version),
                    config=dict(previous.config),
                    created_by=owner_id,
                    created_at=now,
                )
            )
            return self._browser_view(connection, previous)

    def _lock_browser_version(
        self, owner_id: UUID, connection_id: UUID, expected_version: int
    ) -> tuple[SourceConnection, SourceConnectionVersion]:
        connection = self._session.scalar(
            select(SourceConnection)
            .where(SourceConnection.owner_id == owner_id, SourceConnection.id == connection_id)
            .with_for_update()
        )
        if connection is None:
            raise ApplicationError("resource_not_found")
        if connection.current_version != expected_version:
            raise ApplicationError("connection_version_conflict")
        previous = self._session.get(
            SourceConnectionVersion, (connection.id, connection.current_version)
        )
        if previous is None or previous.auth_kind != SourceConnectionAuthKind.BROWSER_STATE.value:
            raise ApplicationError("invalid_connection_configuration")
        return connection, previous

    def _browser_maintenance_time(self) -> datetime:
        now = self._clock()
        if now.tzinfo is None:
            raise ValueError("clock must return a timezone-aware datetime")
        return now.astimezone(UTC)

    def _browser_view(
        self, connection: SourceConnection, version: SourceConnectionVersion
    ) -> SourceConnectionView:
        return SourceConnectionView(
            id=connection.id,
            source_key=connection.source_key,
            status=SourceConnectionStatus(connection.status),
            version=connection.current_version,
            allowed_hosts=list(self._allowed_hosts(version.config)),
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
            allowed_hosts=(
                list(self._allowed_hosts(current_version.config))
                if current_version is not None
                else []
            ),
            capabilities=capability_views,
        )

    @staticmethod
    def _allowed_hosts(config: Mapping[str, object]) -> tuple[str, ...]:
        value = config.get("allowed_hosts", [])
        if not isinstance(value, list) or any(not isinstance(item, str) for item in value):
            raise RuntimeError("connection allowed_hosts config is invalid")
        return tuple(value)


def source_credential_reference(source_key: str, credential: SecretStr) -> str:
    """Return a non-reversible server-only reference; never expose it over HTTP."""
    digest = hashlib.sha256(credential.get_secret_value().encode()).hexdigest()
    return f"settings:{source_key}:{digest}"


@dataclass(frozen=True, slots=True)
class WebConnectionExecution:
    connection_id: UUID
    connection_version: int
    normalized_url: str
    allowed_hosts: frozenset[str]


def resolve_web_connection_execution(
    session: Session,
    *,
    owner_id: UUID,
    target_url: str,
) -> WebConnectionExecution:
    """Resolve and lock the owner's current public-web connection."""
    connection = session.scalar(
        select(SourceConnection)
        .where(
            SourceConnection.owner_id == owner_id,
            SourceConnection.source_key == "web",
        )
        .with_for_update()
    )
    if connection is None:
        raise ApplicationError("resource_not_found")
    return _web_connection_execution(
        session,
        connection=connection,
        connection_version=connection.current_version,
        target_url=target_url,
    )


def require_web_connection_execution(
    session: Session,
    *,
    owner_id: UUID,
    connection_id: UUID,
    connection_version: int,
    target_url: str,
) -> WebConnectionExecution:
    """Lock and validate one public-web execution inside the caller's transaction."""
    connection = session.scalar(
        select(SourceConnection)
        .where(
            SourceConnection.owner_id == owner_id,
            SourceConnection.id == connection_id,
            SourceConnection.source_key == "web",
        )
        .with_for_update()
    )
    if connection is None:
        raise ApplicationError("resource_not_found")
    return _web_connection_execution(
        session,
        connection=connection,
        connection_version=connection_version,
        target_url=target_url,
    )


def _web_connection_execution(
    session: Session,
    *,
    connection: SourceConnection,
    connection_version: int,
    target_url: str,
) -> WebConnectionExecution:
    if connection.status == SourceConnectionStatus.DISABLED.value:
        raise ApplicationError("connection_disabled")
    if connection.current_version != connection_version:
        raise ApplicationError("connection_version_conflict")
    version = session.get(SourceConnectionVersion, (connection.id, connection_version))
    if version is None or version.auth_kind != SourceConnectionAuthKind.NONE.value:
        raise RuntimeError("web connection version is invalid")
    allowed_hosts = frozenset(SourceConnectionService._allowed_hosts(version.config))
    try:
        normalized_url = normalize_web_url(target_url, allowed_hosts=allowed_hosts)
    except ValueError as error:
        raise ApplicationError("source_target_not_allowed") from error
    return WebConnectionExecution(
        connection_id=connection.id,
        connection_version=connection.current_version,
        normalized_url=normalized_url,
        allowed_hosts=allowed_hosts,
    )


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
    if connection is not None and _authentication_failed(session, connection):
        raise ApplicationError("connection_authentication_required")


def pause_bilibili_connection_in_transaction(
    session: Session,
    *,
    owner_id: UUID,
    connection_id: UUID,
    connection_version: int,
    now: datetime,
) -> None:
    """Fence and disable a local crawler login after auth/rate-limit evidence."""
    if not session.in_transaction() or now.tzinfo is None:
        raise RuntimeError("Bilibili pause requires a transaction and aware time")
    connection = session.scalar(
        select(SourceConnection)
        .where(
            SourceConnection.owner_id == owner_id,
            SourceConnection.id == connection_id,
            SourceConnection.source_key == "bilibili",
        )
        .with_for_update()
    )
    if connection is None or connection.current_version != connection_version:
        return
    connection.status = SourceConnectionStatus.DISABLED.value
    connection.updated_at = now.astimezone(UTC)


def require_source_connection_version(
    session: Session,
    *,
    owner_id: UUID,
    source_key: str,
    connection_id: UUID,
    connection_version: int,
) -> SourceConnectionConfig:
    """Fence a source operation and return its immutable non-secret configuration."""
    connection = session.scalar(
        select(SourceConnection)
        .where(
            SourceConnection.owner_id == owner_id,
            SourceConnection.id == connection_id,
            SourceConnection.source_key == source_key,
        )
        .with_for_update()
    )
    if connection is None:
        raise ApplicationError("resource_not_found")
    if connection.status == SourceConnectionStatus.DISABLED.value:
        raise ApplicationError("connection_disabled")
    if connection.current_version != connection_version:
        raise ApplicationError("connection_version_conflict")
    if _authentication_failed(session, connection):
        raise ApplicationError("connection_authentication_required")
    version = session.get(SourceConnectionVersion, (connection_id, connection_version))
    if version is None:
        raise RuntimeError("source connection version is not visible")
    return SourceConnectionConfig.model_validate(version.config)


def require_browser_state_execution(
    session: Session,
    *,
    owner_id: UUID,
    connection_id: UUID,
    connection_version: int,
    store: BrowserStateStore,
) -> StorageState:
    """Resolve only the current, active browser state inside the caller's transaction."""
    connection = session.scalar(
        select(SourceConnection)
        .where(SourceConnection.owner_id == owner_id, SourceConnection.id == connection_id)
        .with_for_update()
    )
    if connection is None:
        raise ApplicationError("resource_not_found")
    if connection.status == SourceConnectionStatus.DISABLED.value:
        raise ApplicationError("connection_disabled")
    if connection.current_version != connection_version:
        raise ApplicationError("connection_version_conflict")
    if _authentication_failed(session, connection):
        raise ApplicationError("connection_authentication_required")
    version = session.get(SourceConnectionVersion, (connection_id, connection_version))
    if (
        version is None
        or version.auth_kind != SourceConnectionAuthKind.BROWSER_STATE.value
        or version.secret_ref is None
    ):
        raise ApplicationError("connection_credentials_missing")
    try:
        return store.load(
            owner_id=owner_id,
            connection_id=connection_id,
            version=connection_version,
            reference=version.secret_ref,
        )
    except BrowserStateError as error:
        raise ApplicationError("connection_credentials_missing") from error


def _authentication_failed(session: Session, connection: SourceConnection) -> bool:
    return (
        session.scalar(
            select(SourceCapabilityEvidence.id)
            .where(
                SourceCapabilityEvidence.owner_id == connection.owner_id,
                SourceCapabilityEvidence.connection_id == connection.id,
                SourceCapabilityEvidence.connection_version == connection.current_version,
                SourceCapabilityEvidence.stop_reason
                == SourceStopReason.AUTHENTICATION_REQUIRED.value,
            )
            .limit(1)
        )
        is not None
    )
