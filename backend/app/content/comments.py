from __future__ import annotations

import hashlib
from collections.abc import Callable, Iterable
from dataclasses import dataclass, field
from datetime import UTC, datetime
from uuid import UUID, uuid5

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from connections.schemas import SourceEntryPoint
from connections.services import require_source_connection_version
from content.models import ContentDiscovery
from content.schemas import CommentCollectionRunInput, PersistContentPostInput
from content.services import ContentService
from evidence.schemas import DataClass
from evidence.services import SourceAccessPolicyService
from jobs.cursor import CursorPageProgress, CursorPageRequest
from jobs.execution import (
    CheckpointConflictError,
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
    BudgetReservationInput,
    CoverageTerminalEvidence,
    CoverageWindowInput,
    CoverageWindowView,
    JobAcceptanceInput,
    JobObservationContext,
    JobStage,
    JobView,
    UsageAttemptInput,
    UsageKind,
    UsageOutcome,
)
from jobs.services import (
    CoverageWindowService,
    JobService,
    ResourceBudgetService,
    load_job_execution_configuration,
)
from sources.contracts import (
    SourceCapability,
    SourceComment,
    SourcePage,
    SourcePageState,
    SourceSort,
)

_COMMENTS_STAGE = "comments.request"
_FIRST_LEVEL_LIMIT = 200
_REPLIES_PER_THREAD_LIMIT = 20


def comment_target_hash(configuration_ref: str, post_external_id: str) -> bytes:
    return hashlib.sha256(f"{configuration_ref}\0{post_external_id}".encode()).digest()


def build_comment_job_acceptance(run: CommentCollectionRunInput) -> JobAcceptanceInput:
    """Freeze the complete execution scope before JobService accepts it."""
    target_hash = comment_target_hash(run.configuration_ref, run.post_external_id)
    return JobAcceptanceInput(
        operation_id=run.operation_id,
        kind="source.comments",
        observation=JobObservationContext(
            configuration_ref=run.configuration_ref,
            configuration_version=run.configuration_version,
            source_key=run.source_key,
            source_capability=SourceCapability.COMMENTS,
        ),
        scheduled_for_at=run.scheduled_for_at,
        scope={
            "connection_id": str(run.connection_id),
            "connection_version": run.connection_version,
            "post_external_id": run.post_external_id,
            "entry_point": run.entry_point.value,
            "sort_key": SourceSort.TOP.value,
            "target_hash": target_hash.hex(),
            "rule_version": run.configuration_version,
            "starts_at": run.starts_at.isoformat(),
            "ends_at": run.ends_at.isoformat(),
            "page_size": run.page_size,
            "max_pages": run.max_pages,
            "max_requests": run.max_requests,
            "max_seconds": run.max_seconds,
            "first_level_limit": _FIRST_LEVEL_LIMIT,
            "replies_per_thread_limit": _REPLIES_PER_THREAD_LIMIT,
            "scan_kind": run.scan_kind.value,
        },
    )


class CommentCollectionAcceptanceService:
    """Accept a frozen comments job for a later scheduler or manual caller."""

    def __init__(
        self,
        session: Session,
        *,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def accept_job(self, *, owner_id: UUID, run: CommentCollectionRunInput) -> JobView:
        return JobService(self._session, clock=self._clock).accept(
            owner_id=owner_id,
            command=build_comment_job_acceptance(run),
        )


class CommentRequestMeter:
    """Reserve and settle every network request made by a comments adapter."""

    def __init__(
        self,
        session: Session,
        *,
        owner_id: UUID,
        lease: ExecutionLease,
        operation_id: UUID,
        source_key: str,
        connection_id: UUID,
        connection_version: int,
        component_key: str,
        max_requests: int,
        deadline_at: datetime,
        lease_seconds: int,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        if not 1 <= max_requests <= 100:
            raise ValueError("comments request budget must be between 1 and 100")
        if deadline_at.utcoffset() is None:
            raise ValueError("comments deadline must be timezone-aware")
        self._session = session
        self._owner_id = owner_id
        self._lease = lease
        self._operation_id = operation_id
        self._source_key = source_key
        self._connection_id = connection_id
        self._connection_version = connection_version
        self._component_key = component_key
        self._max_requests = max_requests
        self._deadline_at = deadline_at
        self._lease_seconds = lease_seconds
        self._clock = clock or (lambda: datetime.now(UTC))
        self._started = 0
        self._pending: list[UUID] = []

    def before_request(self, attempt: int) -> bool:
        if attempt != self._started + 1:
            raise ValueError("source request attempt is out of sequence")
        if self._started >= self._max_requests or self._clock() >= self._deadline_at:
            return False
        execution = JobExecutionService(
            self._session,
            lease_seconds=self._lease_seconds,
            clock=self._clock,
        )
        budget = ResourceBudgetService(self._session, clock=self._clock)
        now = self._clock()
        self._session.rollback()
        try:
            with self._session.begin():
                request_sequence = execution.current_request_count_in_transaction(
                    self._lease,
                    owner_id=self._owner_id,
                    operation_id=self._operation_id,
                )
                if attempt == 1:
                    budget.recover_abandoned_attempts_in_transaction(
                        owner_id=self._owner_id,
                        operation_id=self._operation_id,
                        component_key=self._component_key,
                        stage=_COMMENTS_STAGE,
                        finished_at=now,
                    )
                require_source_connection_version(
                    self._session,
                    owner_id=self._owner_id,
                    source_key=self._source_key,
                    connection_id=self._connection_id,
                    connection_version=self._connection_version,
                )
                SourceAccessPolicyService(
                    self._session,
                    clock=self._clock,
                ).require_admission_ready_in_transaction(
                    owner_id=self._owner_id,
                    source_key=self._source_key,
                    capability=SourceCapability.COMMENTS,
                    data_class=DataClass.STRUCTURED,
                )
                if request_sequence >= self._max_requests:
                    return False
                attempt_id = resource_attempt_id(
                    operation_id=self._operation_id,
                    component_key=self._component_key,
                    stage=_COMMENTS_STAGE,
                    sequence=request_sequence + 1,
                )
                decision = budget.reserve_budget_in_transaction(
                    owner_id=self._owner_id,
                    command=BudgetReservationInput(
                        reservation_id=attempt_id,
                        operation_id=self._operation_id,
                        metric=BudgetMetric.NETWORK_REQUEST,
                        requested_units=1,
                        context=BudgetContext(
                            source_ref=self._source_key,
                            connection_ref=f"connection:{self._connection_id.hex}",
                            job_ref=f"job:{self._lease.job_id.hex}",
                        ),
                    ),
                )
                if decision.status is BudgetDecisionStatus.DELAYED:
                    return False
                usage = budget.begin_attempt_in_transaction(
                    owner_id=self._owner_id,
                    command=UsageAttemptInput(
                        attempt_id=attempt_id,
                        operation_id=self._operation_id,
                        component_key=self._component_key,
                        usage_kind=UsageKind.NETWORK_REQUEST,
                        stage=_COMMENTS_STAGE,
                        started_at=now,
                    ),
                )
                if usage.outcome is not UsageOutcome.STARTED:
                    raise ValueError("source request attempt is already settled")
                renewed, allowed = execution.begin_request_in_transaction(self._lease)
                if not allowed:
                    raise JobLeaseUnavailableError("job cancellation has been requested")
        except JobLeaseUnavailableError:
            return False
        self._lease = renewed
        self._started += 1
        self._pending.append(attempt_id)
        return True

    def settle_page_in_transaction(
        self,
        *,
        page: SourcePage,
        owner_id: UUID,
        lease: ExecutionLease,
        operation_id: UUID,
    ) -> None:
        if (
            owner_id != self._owner_id
            or lease.job_id != self._lease.job_id
            or operation_id != self._operation_id
            or page.request_count != len(self._pending)
        ):
            raise ValueError("source request usage does not match the committed page")
        outcome = (
            UsageOutcome.EMPTY
            if page.state is SourcePageState.EMPTY
            else UsageOutcome.SUCCEEDED
            if page.state in {SourcePageState.MORE, SourcePageState.COMPLETE}
            else UsageOutcome.FAILED
        )
        budget = ResourceBudgetService(self._session, clock=self._clock)
        for attempt_id in self._pending:
            budget.settle_budget_reservation_in_transaction(
                owner_id=owner_id,
                reservation_id=attempt_id,
                actual_units=1,
            )
            budget.finish_attempt_in_transaction(
                owner_id=owner_id,
                attempt_id=attempt_id,
                outcome=outcome,
                finished_at=self._clock(),
            )

    def confirm_page(self, lease: ExecutionLease) -> None:
        self._lease = lease
        self._pending.clear()

    def fail_pending(self) -> None:
        self._session.rollback()
        with self._session.begin():
            budget = ResourceBudgetService(self._session, clock=self._clock)
            for attempt_id in self._pending:
                budget.settle_budget_reservation_in_transaction(
                    owner_id=self._owner_id,
                    reservation_id=attempt_id,
                    actual_units=1,
                )
                budget.finish_attempt_in_transaction(
                    owner_id=self._owner_id,
                    attempt_id=attempt_id,
                    outcome=UsageOutcome.FAILED,
                    finished_at=self._clock(),
                )
        self._pending.clear()


class CommentTreeLimiter:
    """Apply deterministic root and per-root reply limits to a preorder comment tree."""

    def __init__(self, *, first_level_limit: int, replies_per_thread_limit: int) -> None:
        if first_level_limit != _FIRST_LEVEL_LIMIT:
            raise ValueError("first-level comment limit must be 200")
        if replies_per_thread_limit != _REPLIES_PER_THREAD_LIMIT:
            raise ValueError("reply limit must be 20")
        self._first_level_limit = first_level_limit
        self._replies_per_thread_limit = replies_per_thread_limit
        self._seen: set[str] = set()
        self._accepted_roots: set[str] = set()
        self._rejected_roots: set[str] = set()
        self._root_by_comment: dict[str, str] = {}
        self._reply_counts: dict[str, int] = {}

    def admit(self, items: Iterable[SourceComment]) -> tuple[tuple[SourceComment, ...], int]:
        admitted: list[SourceComment] = []
        filtered = 0
        for item in items:
            if item.external_id in self._seen:
                filtered += 1
                continue
            self._seen.add(item.external_id)
            parent_id = item.parent_comment_external_id
            if parent_id is None:
                root_id = item.external_id
                self._root_by_comment[item.external_id] = root_id
                if len(self._accepted_roots) >= self._first_level_limit:
                    self._rejected_roots.add(root_id)
                    filtered += 1
                    continue
                self._accepted_roots.add(root_id)
                admitted.append(item)
                continue

            parent_root_id = self._root_by_comment.get(parent_id)
            if parent_root_id is None:
                resolved_root_id = parent_id
                self._root_by_comment[parent_id] = resolved_root_id
                if len(self._accepted_roots) >= self._first_level_limit:
                    self._rejected_roots.add(resolved_root_id)
                else:
                    self._accepted_roots.add(resolved_root_id)
            else:
                resolved_root_id = parent_root_id
            self._root_by_comment[item.external_id] = resolved_root_id
            if resolved_root_id in self._rejected_roots:
                filtered += 1
                continue
            count = self._reply_counts.get(resolved_root_id, 0)
            if count >= self._replies_per_thread_limit:
                filtered += 1
                continue
            self._reply_counts[resolved_root_id] = count + 1
            admitted.append(item)
        return tuple(admitted), filtered


@dataclass(frozen=True, slots=True)
class CommentPageCommitResult:
    lease: ExecutionLease
    coverage: CoverageWindowView
    saved_items: int
    filtered_items: int
    progress: CursorPageProgress = field(repr=False)


class CommentPageCommitService:
    """Commit one admitted comments page, usage and cursor in one transaction."""

    def __init__(
        self,
        session: Session,
        *,
        lease_seconds: int,
        first_level_limit: int = _FIRST_LEVEL_LIMIT,
        replies_per_thread_limit: int = _REPLIES_PER_THREAD_LIMIT,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._session = session
        self._lease_seconds = lease_seconds
        self._clock = clock or (lambda: datetime.now(UTC))
        self._limiter = CommentTreeLimiter(
            first_level_limit=first_level_limit,
            replies_per_thread_limit=replies_per_thread_limit,
        )

    def commit_page(
        self,
        *,
        owner_id: UUID,
        lease: ExecutionLease,
        window: CoverageWindowInput,
        request: CursorPageRequest,
        page_operation_id: UUID,
        connection_id: UUID,
        connection_version: int,
        page: SourcePage,
        meter: CommentRequestMeter | None = None,
    ) -> CommentPageCommitResult:
        configuration = load_job_execution_configuration(self._session, job_id=lease.job_id)
        if (
            configuration is None
            or configuration.owner_id != owner_id
            or configuration.kind != "source.comments"
            or configuration.observation.source_key != window.source_key
            or configuration.observation.source_capability is not SourceCapability.COMMENTS
            or configuration.observation.configuration_version != window.rule_version
            or configuration.scope.get("connection_id") != str(connection_id)
            or configuration.scope.get("connection_version") != connection_version
            or configuration.scope.get("target_hash") != window.target_hash.hex()
            or configuration.scope.get("rule_version") != window.rule_version
            or configuration.scope.get("sort_key") != window.sort_key.value
            or configuration.scope.get("starts_at") != window.starts_at.isoformat()
            or configuration.scope.get("ends_at") != window.ends_at.isoformat()
            or configuration.scope.get("max_pages") != request.max_pages
            or configuration.scope.get("first_level_limit") != _FIRST_LEVEL_LIMIT
            or configuration.scope.get("replies_per_thread_limit") != _REPLIES_PER_THREAD_LIMIT
            or window.owner_id != owner_id
            or window.capability is not SourceCapability.COMMENTS
            or page.source_key != window.source_key
            or page.capability is not SourceCapability.COMMENTS
        ):
            self._session.rollback()
            raise ValueError("comment page does not match the accepted comments scope")
        post_external_id = configuration.scope.get("post_external_id")
        if (
            not isinstance(post_external_id, str)
            or comment_target_hash(
                configuration.observation.configuration_ref,
                post_external_id,
            )
            != window.target_hash
        ):
            self._session.rollback()
            raise ValueError("comment target hash does not match the accepted scope")
        entry_point_value = configuration.scope.get("entry_point")
        if not isinstance(entry_point_value, str):
            self._session.rollback()
            raise ValueError("comment entry point does not match the accepted scope")
        entry_point = SourceEntryPoint(entry_point_value)

        candidates: list[SourceComment] = []
        for item in page.items:
            if not isinstance(item, SourceComment) or item.post_external_id != post_external_id:
                self._session.rollback()
                raise ValueError("comments page can contain only comments for the accepted post")
            candidates.append(item)
        admitted, filtered_items = self._limiter.admit(candidates)

        self._session.rollback()
        execution = JobExecutionService(
            self._session,
            lease_seconds=self._lease_seconds,
            clock=self._clock,
        )
        policy = SourceAccessPolicyService(self._session, clock=self._clock)
        with self._session.begin():
            current = execution.require_current_operation_in_transaction(
                lease,
                owner_id=owner_id,
                operation_id=configuration.operation_id,
            )
            if (
                current.checkpoint_sequence != lease.checkpoint_sequence
                or current.checkpoint != lease.checkpoint
            ):
                raise CheckpointConflictError("comment page belongs to an older checkpoint")
            require_source_connection_version(
                self._session,
                owner_id=owner_id,
                source_key=window.source_key,
                connection_id=connection_id,
                connection_version=connection_version,
            )
            policy.require_admission_ready_in_transaction(
                owner_id=owner_id,
                source_key=window.source_key,
                capability=SourceCapability.COMMENTS,
                data_class=DataClass.STRUCTURED,
            )
            content = ContentService(self._session, clock=self._clock)
            for item in admitted:
                admission = policy.admit_payload_in_transaction(
                    owner_id=owner_id,
                    source_key=window.source_key,
                    capability=SourceCapability.COMMENTS,
                    data_class=DataClass.STRUCTURED,
                    collected_at=page.observed_at,
                    payload=self._payload(item),
                )
                content.persist_comment_in_transaction(
                    owner_id=owner_id,
                    command=PersistContentPostInput(
                        job_id=lease.job_id,
                        source_operation_id=uuid5(page_operation_id, item.external_id),
                        connection_id=connection_id,
                        connection_version=connection_version,
                        entry_point=entry_point,
                        component_name=f"{window.source_key}.comments",
                        component_version=page.adapter_version or "unknown",
                        admission=admission,
                    ),
                )
            saved_items = self._session.scalar(
                select(func.count(ContentDiscovery.id)).where(
                    ContentDiscovery.owner_id == owner_id,
                    ContentDiscovery.job_id == lease.job_id,
                )
            )
            assert saved_items is not None
            terminal_evidence = (
                CoverageTerminalEvidence(
                    starts_at=window.starts_at,
                    ends_at=window.ends_at,
                    sort_key=window.sort_key,
                    query_bounded=True,
                    sort_applied=True,
                    terminal_verified=True,
                )
                if page.state in {SourcePageState.COMPLETE, SourcePageState.EMPTY}
                else None
            )
            renewed, coverage, progress = CoverageWindowService(
                self._session,
                execution=execution,
                clock=self._clock,
            ).record_cursor_page_in_transaction(
                lease=lease,
                window=window,
                request=request,
                page_state=page.state,
                next_token=page.next_page_token,
                stop_reason=page.stop_reason,
                evidence=terminal_evidence,
                job_progress=JobProgress(stage=JobStage.SAVE, items_saved=saved_items),
            )
            if meter is not None:
                meter.settle_page_in_transaction(
                    page=page,
                    owner_id=owner_id,
                    lease=lease,
                    operation_id=configuration.operation_id,
                )
        if meter is not None:
            meter.confirm_page(renewed)
        return CommentPageCommitResult(
            lease=renewed,
            coverage=coverage,
            saved_items=saved_items,
            filtered_items=filtered_items,
            progress=progress,
        )

    @staticmethod
    def _payload(comment: SourceComment) -> dict[str, object]:
        fields: dict[str, object] = {
            "object_type": "comment",
            "external_id": comment.external_id,
            "post_external_id": comment.post_external_id,
            "parent_comment_external_id": comment.parent_comment_external_id,
            "published_at": (
                comment.published_at.isoformat() if comment.published_at is not None else None
            ),
            "like_count": comment.like_count,
        }
        if comment.author_external_id is not None:
            fields["author_external_id"] = comment.author_external_id
        if comment.author_name is not None:
            fields["author_name"] = comment.author_name
        if comment.canonical_url is not None:
            fields["canonical_url"] = comment.canonical_url
        if comment.text is not None:
            fields.update(
                body=comment.text,
                text_scope="full",
                text_origin="source",
            )
        return fields
