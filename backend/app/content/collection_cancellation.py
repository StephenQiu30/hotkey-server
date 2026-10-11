from datetime import datetime

from sqlalchemy.orm import Session

from jobs.execution import ExecutionLease, JobExecutionService
from jobs.schemas import CollectionScanKind, CoverageWindowInput
from jobs.services import (
    CoverageWindowService,
    ResourceBudgetService,
    load_job_execution_configuration,
)
from sources.contracts import SourceSort, SourceStopReason


def recover_interrupted_collection_in_transaction(
    session: Session,
    *,
    execution: JobExecutionService,
    lease: ExecutionLease,
    finished_at: datetime,
) -> None:
    """Finalize collection state even when the supervisor terminates the child."""
    lease = execution.require_current_lease_allowing_cancel_in_transaction(lease)
    configuration = load_job_execution_configuration(session, job_id=lease.job_id)
    if configuration is None or configuration.kind not in {"keyword.search", "source.comments"}:
        return
    source_key = configuration.observation.source_key
    if source_key is None:
        return
    ResourceBudgetService(
        session, clock=lambda: finished_at
    ).recover_abandoned_attempts_in_transaction(
        owner_id=configuration.owner_id,
        operation_id=configuration.operation_id,
        component_key=f"collector.{source_key}",
        stage="search.request" if configuration.kind == "keyword.search" else "comments.request",
        finished_at=finished_at,
    )
    if not execution.cancellation_requested_in_transaction(lease):
        return
    scope = configuration.scope
    scan_kind = scope.get("scan_kind")
    if not isinstance(scan_kind, str) or scan_kind not in CollectionScanKind:
        return
    try:
        target, sort, starts_at, ends_at = (
            scope[key] for key in ("target_hash", "sort_key", "starts_at", "ends_at")
        )
        if not all(isinstance(value, str) for value in (target, sort, starts_at, ends_at)):
            return
        assert isinstance(target, str) and isinstance(sort, str)
        assert isinstance(starts_at, str) and isinstance(ends_at, str)
        window = CoverageWindowInput.model_validate(
            {
                "owner_id": configuration.owner_id,
                "source_key": source_key,
                "capability": configuration.observation.source_capability,
                "target_hash": bytes.fromhex(target),
                "sort_key": SourceSort(sort),
                "rule_version": scope["rule_version"],
                "starts_at": datetime.fromisoformat(starts_at),
                "ends_at": datetime.fromisoformat(ends_at),
            }
        )
    except (KeyError, ValueError):
        # Malformed tasks can still be cancelled without inventing coverage facts.
        return
    coverage = CoverageWindowService(session, execution=execution, clock=lambda: finished_at)
    opened = coverage.begin_in_transaction(lease=lease, window=window, allow_cancel=True)
    if opened.status != "confirmed":
        coverage.mark_partial_without_page_in_transaction(
            lease=lease,
            window=window,
            stop_reason=SourceStopReason.CANCELLED,
            allow_cancel=True,
        )
