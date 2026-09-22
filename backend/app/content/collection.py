from __future__ import annotations

from collections.abc import Callable
from contextlib import AbstractContextManager
from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy.orm import Session

from connections.services import require_web_connection_execution
from content.schemas import ContentRecordDetailView, PersistContentDocumentInput
from content.services import ContentService
from core.errors import ApplicationError
from evidence.services import LifecycleService
from jobs.execution import (
    ExecutionLease,
    JobExecutionService,
    JobLeaseUnavailableError,
    JobProgress,
    resource_attempt_id,
)
from jobs.schemas import (
    BudgetContext,
    BudgetDecisionStatus,
    BudgetMetric,
    BudgetReservationDecision,
    BudgetReservationInput,
    JobStage,
    UsageAttemptInput,
    UsageKind,
    UsageOutcome,
)
from jobs.services import ResourceBudgetService, UsageConflictError
from sources.adapters.web_targets import normalize_web_url
from sources.contracts import (
    DocumentAdapter,
    SourceStopReason,
    WebPageRequest,
    WebPageResult,
)

type Clock = Callable[[], datetime]
type DocumentAdapterFactory = Callable[[frozenset[str]], AbstractContextManager[DocumentAdapter]]

_FIRECRAWL_COMPONENT_KEY = "collector.firecrawl"
_FIRECRAWL_STAGE = "page_content.fetch"


@dataclass(frozen=True, slots=True)
class WebPageFetchInput:
    operation_id: UUID
    connection_id: UUID
    connection_version: int
    target_url: str

    def __post_init__(self) -> None:
        if self.connection_version < 1:
            raise ValueError("connection_version must be positive")


@dataclass(frozen=True, slots=True)
class BudgetedWebPageResult:
    result: WebPageResult
    lease: ExecutionLease
    attempt_id: UUID


class WebPageBudgetDelayedError(RuntimeError):
    def __init__(self, decision: BudgetReservationDecision) -> None:
        super().__init__("collector_call budget is exhausted")
        self.decision = decision


class WebPageFetchService:
    """Fence, meter, and execute one public-web collector call."""

    def __init__(
        self,
        session: Session,
        *,
        lease_seconds: int,
        clock: Clock | None = None,
    ) -> None:
        self._session = session
        self._lease_seconds = lease_seconds
        self._clock = clock or (lambda: datetime.now(UTC))

    def fetch_document(
        self,
        *,
        owner_id: UUID,
        lease: ExecutionLease,
        command: WebPageFetchInput,
        adapter_factory: DocumentAdapterFactory,
    ) -> BudgetedWebPageResult:
        started_at = self._clock()
        attempt_id = resource_attempt_id(
            operation_id=command.operation_id,
            component_key=_FIRECRAWL_COMPONENT_KEY,
            stage=_FIRECRAWL_STAGE,
            sequence=lease.epoch,
        )
        execution = JobExecutionService(
            self._session,
            lease_seconds=self._lease_seconds,
            clock=self._clock,
        )
        budget = ResourceBudgetService(self._session, clock=self._clock)

        self._session.rollback()
        with self._session.begin():
            execution.require_current_operation_in_transaction(
                lease,
                owner_id=owner_id,
                operation_id=command.operation_id,
            )
            connection = require_web_connection_execution(
                self._session,
                owner_id=owner_id,
                connection_id=command.connection_id,
                connection_version=command.connection_version,
                target_url=command.target_url,
            )
            decision = budget.reserve_budget_in_transaction(
                owner_id=owner_id,
                command=BudgetReservationInput(
                    reservation_id=attempt_id,
                    operation_id=command.operation_id,
                    metric=BudgetMetric.COLLECTOR_CALL,
                    requested_units=1,
                    context=BudgetContext(
                        source_ref="web",
                        connection_ref=f"connection:{command.connection_id.hex}",
                        job_ref=f"job:{lease.job_id.hex}",
                    ),
                ),
            )
            if decision.status is BudgetDecisionStatus.DELAYED:
                raise WebPageBudgetDelayedError(decision)
            attempt = budget.begin_attempt_in_transaction(
                owner_id=owner_id,
                command=UsageAttemptInput(
                    attempt_id=attempt_id,
                    operation_id=command.operation_id,
                    component_key=_FIRECRAWL_COMPONENT_KEY,
                    usage_kind=UsageKind.COLLECTOR_CALL,
                    stage=_FIRECRAWL_STAGE,
                    started_at=started_at,
                ),
            )
            if attempt.outcome is not UsageOutcome.STARTED:
                raise UsageConflictError("collector attempt is already settled")
            renewed, request_allowed = execution.begin_request_in_transaction(lease)
            if not request_allowed:
                raise JobLeaseUnavailableError("job cancellation has been requested")

        request = WebPageRequest(url=connection.normalized_url)
        call_started = False
        try:
            with adapter_factory(connection.allowed_hosts) as adapter:
                call_started = True
                result = adapter.fetch_document(request)
        except Exception:
            self._settle(
                budget,
                owner_id=owner_id,
                attempt_id=attempt_id,
                actual_units=int(call_started),
                outcome=UsageOutcome.FAILED,
            )
            raise

        self._settle(
            budget,
            owner_id=owner_id,
            attempt_id=attempt_id,
            actual_units=result.collector_call_count,
            outcome=self._usage_outcome(result),
        )
        return BudgetedWebPageResult(
            result=result,
            lease=renewed,
            attempt_id=attempt_id,
        )

    def _settle(
        self,
        budget: ResourceBudgetService,
        *,
        owner_id: UUID,
        attempt_id: UUID,
        actual_units: int,
        outcome: UsageOutcome,
    ) -> None:
        finished_at = self._clock()
        self._session.rollback()
        with self._session.begin():
            budget.settle_budget_reservation_in_transaction(
                owner_id=owner_id,
                reservation_id=attempt_id,
                actual_units=actual_units,
            )
            budget.finish_attempt_in_transaction(
                owner_id=owner_id,
                attempt_id=attempt_id,
                outcome=outcome,
                finished_at=finished_at,
            )

    @staticmethod
    def _usage_outcome(result: WebPageResult) -> UsageOutcome:
        if result.document is not None:
            return UsageOutcome.SUCCEEDED
        if result.collector_call_count == 0:
            return UsageOutcome.FILTERED
        if result.stop_reason in {
            SourceStopReason.SOURCE_EMPTY,
            SourceStopReason.NOT_FOUND,
        }:
            return UsageOutcome.EMPTY
        return UsageOutcome.FAILED


class WebPageCommitService:
    """Commit a validated web document and its durable checkpoint atomically."""

    def __init__(
        self,
        session: Session,
        *,
        lease_seconds: int,
        clock: Clock | None = None,
    ) -> None:
        self._session = session
        self._lease_seconds = lease_seconds
        self._clock = clock or (lambda: datetime.now(UTC))

    def commit_document_page(
        self,
        *,
        owner_id: UUID,
        lease: ExecutionLease,
        command: PersistContentDocumentInput,
        sequence: int,
    ) -> tuple[ContentRecordDetailView, ExecutionLease]:
        fields = dict(command.admission.fields)
        request_url = fields.get("request_url")
        final_url = fields.get("final_url")
        if not isinstance(request_url, str) or not isinstance(final_url, str):
            raise ValueError("document URLs must be admitted as strings")

        execution = JobExecutionService(
            self._session,
            lease_seconds=self._lease_seconds,
            clock=self._clock,
        )
        self._session.rollback()
        with self._session.begin():
            execution.require_current_lease_in_transaction(lease)
            connection = require_web_connection_execution(
                self._session,
                owner_id=owner_id,
                connection_id=command.connection_id,
                connection_version=command.connection_version,
                target_url=request_url,
            )
            try:
                normalized_final_url = normalize_web_url(
                    final_url,
                    allowed_hosts=connection.allowed_hosts,
                )
            except ValueError as error:
                raise ApplicationError("source_target_not_allowed") from error
            LifecycleService(
                self._session,
                clock=self._clock,
            ).require_admission_in_transaction(
                owner_id=owner_id,
                admission=command.admission,
            )
            fields["request_url"] = connection.normalized_url
            fields["final_url"] = normalized_final_url
            normalized = command.model_copy(
                update={"admission": command.admission.model_copy(update={"fields": fields})}
            )
            content = ContentService(
                self._session,
                clock=self._clock,
            ).persist_document_in_transaction(
                owner_id=owner_id,
                command=normalized,
            )
            renewed = execution.save_checkpoint_in_transaction(
                lease,
                sequence=sequence,
                checkpoint={
                    "content_id": str(content.id),
                    "observation_id": str(content.latest_observation.id),
                },
                progress=JobProgress(stage=JobStage.SAVE, items_saved=1),
            )
        return content, renewed
