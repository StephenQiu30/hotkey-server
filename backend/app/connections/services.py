from __future__ import annotations

from collections.abc import Callable, Iterable
from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy import and_, select
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
    SourceCapabilityStatus,
    SourceCapabilityView,
    SourceConnectionStatus,
    SourceEntryPoint,
    SourceEntryPointView,
    SourcePlatformStatus,
    SourcePlatformView,
    SourceRolloutRole,
)
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
    def __init__(self, session: Session, *, clock: Clock | None = None) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

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
            for evidence in evidence_rows:
                key = (evidence.connection_id, evidence.capability, evidence.entry_point)
                latest[key] = evidence
                if (
                    evidence.kind == ConnectionEvidenceKind.PERSISTED_READ.value
                    and evidence.outcome == ConnectionEvidenceOutcome.SUCCEEDED.value
                ):
                    last_success[key] = evidence.observed_at

            return [
                self._platform_view(
                    catalog=catalog,
                    connection=connections.get(catalog.source_key),
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
        policy_readiness: dict[tuple[str, SourceCapability], bool],
        latest: dict[tuple[UUID, str, str], SourceCapabilityEvidence],
        last_success: dict[tuple[UUID, str, str], datetime],
    ) -> SourcePlatformView:
        capability_views: list[SourceCapabilityView] = []
        statuses: list[SourceCapabilityStatus] = []
        for capability in SourceCapability:
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
            has_credentials=connection is not None,
            capabilities=capability_views,
        )
