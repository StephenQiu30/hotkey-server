from __future__ import annotations

from datetime import UTC, datetime

import pytest

from connections.schemas import (
    ConnectionEvidenceKind,
    ConnectionEvidenceOutcome,
    SourceCapabilityStatus,
    SourceConnectionStatus,
    SourcePlatformStatus,
)
from connections.services import (
    CapabilityStatusFacts,
    aggregate_platform_status,
    resolve_capability_status,
)
from sources.contracts import SourceStopReason


@pytest.mark.parametrize(
    ("facts", "expected"),
    [
        (
            CapabilityStatusFacts(product_restricted=True),
            SourceCapabilityStatus.RESTRICTED,
        ),
        (CapabilityStatusFacts(), SourceCapabilityStatus.UNCONFIGURED),
        (
            CapabilityStatusFacts(connection_status=SourceConnectionStatus.DISABLED),
            SourceCapabilityStatus.DISABLED,
        ),
        (
            CapabilityStatusFacts(
                connection_status=SourceConnectionStatus.ACTIVE,
                policy_ready=False,
            ),
            SourceCapabilityStatus.RESTRICTED,
        ),
        (
            CapabilityStatusFacts(
                connection_status=SourceConnectionStatus.ACTIVE,
                policy_ready=True,
                evidence_kind=ConnectionEvidenceKind.PROBE,
                evidence_outcome=ConnectionEvidenceOutcome.SUCCEEDED,
            ),
            SourceCapabilityStatus.PENDING_VERIFICATION,
        ),
        (
            CapabilityStatusFacts(
                connection_status=SourceConnectionStatus.ACTIVE,
                policy_ready=True,
                evidence_kind=ConnectionEvidenceKind.PERSISTED_READ,
                evidence_outcome=ConnectionEvidenceOutcome.SUCCEEDED,
            ),
            SourceCapabilityStatus.AVAILABLE,
        ),
        (
            CapabilityStatusFacts(
                connection_status=SourceConnectionStatus.ACTIVE,
                policy_ready=True,
                evidence_kind=ConnectionEvidenceKind.PERSISTED_READ,
                evidence_outcome=ConnectionEvidenceOutcome.FAILED,
                stop_reason=SourceStopReason.AUTHENTICATION_REQUIRED,
            ),
            SourceCapabilityStatus.AUTHENTICATION_REQUIRED,
        ),
        (
            CapabilityStatusFacts(
                connection_status=SourceConnectionStatus.ACTIVE,
                policy_ready=True,
                evidence_kind=ConnectionEvidenceKind.PERSISTED_READ,
                evidence_outcome=ConnectionEvidenceOutcome.FAILED,
                stop_reason=SourceStopReason.ACCESS_DENIED,
            ),
            SourceCapabilityStatus.RESTRICTED,
        ),
    ],
)
def test_capability_status_uses_frozen_priority(
    facts: CapabilityStatusFacts,
    expected: SourceCapabilityStatus,
) -> None:
    assert resolve_capability_status(facts) is expected


def test_transient_failure_stays_pending_without_expiring_old_success_timestamp() -> None:
    last_success = datetime(2026, 9, 22, 8, tzinfo=UTC)
    facts = CapabilityStatusFacts(
        connection_status=SourceConnectionStatus.ACTIVE,
        policy_ready=True,
        evidence_kind=ConnectionEvidenceKind.PERSISTED_READ,
        evidence_outcome=ConnectionEvidenceOutcome.FAILED,
        stop_reason=SourceStopReason.UPSTREAM_ERROR,
        last_persisted_success_at=last_success,
    )

    assert resolve_capability_status(facts) is SourceCapabilityStatus.PENDING_VERIFICATION
    assert facts.last_persisted_success_at == last_success


def test_partial_is_only_a_platform_aggregate() -> None:
    assert (
        aggregate_platform_status(
            [
                SourceCapabilityStatus.AVAILABLE,
                SourceCapabilityStatus.RESTRICTED,
            ]
        )
        is SourcePlatformStatus.PARTIAL
    )
    assert (
        aggregate_platform_status([SourceCapabilityStatus.UNCONFIGURED] * 8)
        is SourcePlatformStatus.UNCONFIGURED
    )
