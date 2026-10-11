from __future__ import annotations

import hashlib
from collections.abc import Callable, Iterable
from dataclasses import dataclass, field
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid5

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from connections.schemas import SourceEntryPoint
from connections.services import (
    load_applied_source_presets_in_transaction,
    load_execution_policy_in_transaction,
    require_source_connection_version,
)
from content.models import ContentDiscovery, ContentRecord, ContentVersion
from content.schemas import (
    CommentCollectionRunInput,
    CommentManualRunInput,
    CommentRunReadinessView,
    PersistContentPostInput,
)
from content.services import ContentService
from core.errors import ApplicationError
from evidence.schemas import DataClass
from evidence.services import SourceAccessPolicyService, load_source_access_readiness
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
    BudgetScopeKind,
    BudgetWindowUsageView,
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
    has_recent_comment_job_target_in_transaction,
    load_content_job_contexts,
    load_job_execution_configuration,
    load_job_execution_configuration_by_operation,
)
from monitors.services import MonitorScheduleService, evaluate_monitor_rules
from sources.contracts import (
    SourceCapability,
    SourceComment,
    SourcePage,
    SourcePageState,
    SourceSort,
    SourceStopReason,
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
            "first_level_limit": run.first_level_limit,
            "replies_per_thread_limit": run.replies_per_thread_limit,
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

    def accept_job_in_transaction(
        self, *, owner_id: UUID, run: CommentCollectionRunInput, manual_content_id: UUID
    ) -> JobView:
        if not self._session.in_transaction():
            raise RuntimeError("comment acceptance requires the caller's transaction")
        acceptance = build_comment_job_acceptance(run)
        acceptance = acceptance.model_copy(
            update={"scope": {**acceptance.scope, "manual_content_id": str(manual_content_id)}}
        )
        return JobService(self._session, clock=self._clock).accept_in_transaction(
            owner_id=owner_id, command=acceptance
        )


@dataclass(frozen=True, slots=True)
class CommentManualRunResult:
    job_id: UUID
    replayed: bool


class CommentManualRunService:
    """Accept a visible HN post's comments refresh under one owner transaction."""

    def __init__(self, session: Session, *, clock: Callable[[], datetime] | None = None) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def run(
        self, *, owner_id: UUID, content_id: UUID, command: CommentManualRunInput
    ) -> CommentManualRunResult:
        now = self._clock()
        if now.utcoffset() is None:
            raise ValueError("manual comments time must be timezone-aware")
        now = now.astimezone(UTC)
        self._session.rollback()
        with self._session.begin():
            lock_key = int.from_bytes(command.operation_id.bytes[:8], "big", signed=True)
            self._session.scalar(select(func.pg_advisory_xact_lock(lock_key)))
            post, version = ContentService(
                self._session
            ).readable_post_for_comment_run_in_transaction(
                owner_id=owner_id, content_id=content_id, now=now
            )
            if post.object_type != "post" or post.source_key != "hackernews":
                raise ApplicationError("comments_not_ready")
            if version is None:
                raise ApplicationError("resource_not_found")
            previous = load_job_execution_configuration_by_operation(
                self._session,
                owner_id=owner_id,
                kind="source.comments",
                operation_id=command.operation_id,
            )
            if previous is not None:
                if previous.scope.get("manual_content_id") != str(content_id):
                    raise ApplicationError("idempotency_conflict")
                return CommentManualRunResult(job_id=previous.job_id, replayed=True)

            run = self._prepare_run(
                owner_id=owner_id,
                content_id=content_id,
                post=post,
                version=version,
                now=now,
                operation_id=command.operation_id,
            )
            job = CommentCollectionAcceptanceService(
                self._session, clock=lambda: now
            ).accept_job_in_transaction(owner_id=owner_id, run=run, manual_content_id=content_id)
            return CommentManualRunResult(job_id=job.id, replayed=False)

    def readiness(self, *, owner_id: UUID, content_id: UUID) -> CommentRunReadinessView:
        now = self._clock()
        if now.utcoffset() is None:
            raise ValueError("manual comments time must be timezone-aware")
        now = now.astimezone(UTC)
        self._session.rollback()
        with self._session.begin():
            post, version = ContentService(
                self._session
            ).readable_post_for_comment_run_in_transaction(
                owner_id=owner_id, content_id=content_id, now=now
            )
            try:
                self._prepare_run(
                    owner_id=owner_id,
                    content_id=content_id,
                    post=post,
                    version=version,
                    now=now,
                    operation_id=UUID(int=0),
                )
            except ApplicationError as error:
                if error.code not in {
                    "comments_not_ready",
                    "comments_budget_exhausted",
                    "comments_rate_limited",
                }:
                    raise
                return CommentRunReadinessView(
                    supported=error.code != "comments_not_ready",
                    available=False,
                    reason=error.code,
                )
            return CommentRunReadinessView(supported=True, available=True, reason=None)

    def _prepare_run(
        self,
        *,
        owner_id: UUID,
        content_id: UUID,
        post: ContentRecord,
        version: ContentVersion | None,
        now: datetime,
        operation_id: UUID,
    ) -> CommentCollectionRunInput:
        if post.object_type != "post" or post.source_key != "hackernews":
            raise ApplicationError("comments_not_ready")
        if version is None:
            raise ApplicationError("resource_not_found")
        searchable_text = "\n".join(item for item in (version.title, version.body) if item)
        discovered_job_ids = tuple(
            self._session.scalars(
                select(ContentDiscovery.job_id).where(
                    ContentDiscovery.owner_id == owner_id,
                    ContentDiscovery.content_id == content_id,
                )
            )
        )
        discovered_topics = {
            context.configuration_ref
            for context in load_content_job_contexts(
                self._session, owner_id=owner_id, job_ids=set(discovered_job_ids)
            ).values()
            if context.configuration_ref.startswith("topic:")
        }
        topic = next(
            (
                candidate
                for candidate in MonitorScheduleService(
                    self._session
                ).list_active_topics_for_scanning_in_transaction()
                if candidate.owner_id == owner_id
                and post.source_key in candidate.source_keys
                and f"topic:{candidate.topic_id}" in discovered_topics
                and evaluate_monitor_rules(candidate.rules, searchable_text).matched
            ),
            None,
        )
        if topic is None:
            raise ApplicationError("comments_not_ready")
        preset = load_applied_source_presets_in_transaction(
            self._session, owner_id=owner_id, source_keys=(post.source_key,)
        ).get(post.source_key)
        if (
            preset is None
            or SourceCapability.COMMENTS not in preset.capabilities
            or preset.comment_scan_policy is None
            or not load_source_access_readiness(self._session, owner_id=owner_id, now=now).get(
                (post.source_key, SourceCapability.COMMENTS), False
            )
        ):
            raise ApplicationError("comments_not_ready")
        policy = load_execution_policy_in_transaction(
            self._session,
            owner_id=owner_id,
            connection_id=preset.connection_id,
            connection_version=preset.connection_version,
        )
        if not policy.enabled or policy.quiet_at(now):
            raise ApplicationError("comments_not_ready")
        budgets = ResourceBudgetService(self._session, clock=lambda: now).budget_usage_snapshot(
            owner_id=owner_id
        )
        if not self._has_budget(budgets, post.source_key):
            raise ApplicationError("comments_budget_exhausted")
        scan = preset.comment_scan_policy
        recent = has_recent_comment_job_target_in_transaction(
            self._session,
            owner_id=owner_id,
            source_key=post.source_key,
            post_external_id=post.external_id,
            since=now - timedelta(seconds=scan.refresh_interval_seconds),
        )
        if recent:
            raise ApplicationError("comments_rate_limited")
        run = CommentCollectionRunInput(
            operation_id=operation_id,
            configuration_ref=f"topic:{topic.topic_id}",
            configuration_version=topic.topic_version,
            source_key=post.source_key,
            connection_id=preset.connection_id,
            connection_version=preset.connection_version,
            post_external_id=post.external_id,
            entry_point=SourceEntryPoint.MANUAL,
            starts_at=now - timedelta(seconds=scan.refresh_interval_seconds),
            ends_at=now,
            scheduled_for_at=now,
            page_size=scan.page_size,
            max_pages=scan.max_pages,
            max_requests=scan.max_requests,
            max_seconds=scan.max_seconds,
            first_level_limit=scan.first_level_limit,
            replies_per_thread_limit=scan.replies_per_thread_limit,
        )
        return run

    @staticmethod
    def _has_budget(budgets: tuple[BudgetWindowUsageView, ...], source_key: str) -> bool:
        relevant = [
            item
            for item in budgets
            if item.metric is BudgetMetric.NETWORK_REQUEST
            and (
                item.scope_kind is BudgetScopeKind.GLOBAL
                or (
                    item.scope_kind is BudgetScopeKind.SOURCE and item.scope_reference == source_key
                )
            )
        ]
        return {BudgetScopeKind.GLOBAL, BudgetScopeKind.SOURCE}.issubset(
            {item.scope_kind for item in relevant}
        ) and all(
            item.enabled and item.remaining_units is not None and item.remaining_units > 0
            for item in relevant
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
                if execution.cancellation_requested_in_transaction(self._lease):
                    return False
                request_counts = execution.current_request_counts_in_transaction(
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
                if request_counts.collection_cycle >= self._max_requests:
                    return False
                attempt_id = resource_attempt_id(
                    operation_id=self._operation_id,
                    component_key=self._component_key,
                    stage=_COMMENTS_STAGE,
                    sequence=request_counts.total + 1,
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
        if not 1 <= first_level_limit <= _FIRST_LEVEL_LIMIT:
            raise ValueError("first-level comment limit must be between 1 and 200")
        if not 0 <= replies_per_thread_limit <= _REPLIES_PER_THREAD_LIMIT:
            raise ValueError("reply limit must be between 0 and 20")
        self._first_level_limit = first_level_limit
        self._replies_per_thread_limit = replies_per_thread_limit
        self._seen: set[str] = set()
        self._accepted_roots: set[str] = set()
        self._rejected_roots: set[str] = set()
        self._root_by_comment: dict[str, str] = {}
        self._reply_counts: dict[str, int] = {}
        self._truncated = False

    @property
    def truncated(self) -> bool:
        return self._truncated

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
                if root_id in self._rejected_roots or (
                    root_id not in self._accepted_roots
                    and len(self._accepted_roots) >= self._first_level_limit
                ):
                    self._rejected_roots.add(root_id)
                    self._truncated = True
                    filtered += 1
                    continue
                self._accepted_roots.add(root_id)
                admitted.append(item)
                continue

            parent_root_id = self._root_by_comment.get(parent_id)
            source_root_id = item.root_comment_external_id
            if (
                source_root_id is not None
                and parent_root_id is not None
                and source_root_id != parent_root_id
            ):
                raise ValueError("comment root conflicts with the known parent chain")
            if parent_root_id is None:
                resolved_root_id = source_root_id or parent_id
                self._root_by_comment[parent_id] = resolved_root_id
            else:
                resolved_root_id = parent_root_id
            self._root_by_comment[item.external_id] = resolved_root_id
            if resolved_root_id not in self._accepted_roots | self._rejected_roots:
                if len(self._accepted_roots) >= self._first_level_limit:
                    self._rejected_roots.add(resolved_root_id)
                else:
                    self._accepted_roots.add(resolved_root_id)
            if resolved_root_id in self._rejected_roots:
                self._truncated = True
                filtered += 1
                continue
            count = self._reply_counts.get(resolved_root_id, 0)
            if count >= self._replies_per_thread_limit:
                self._truncated = True
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
        self._first_level_limit = first_level_limit
        self._replies_per_thread_limit = replies_per_thread_limit
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
            or configuration.scope.get("first_level_limit") != self._first_level_limit
            or configuration.scope.get("replies_per_thread_limit") != self._replies_per_thread_limit
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
            page_state = page.state
            stop_reason = page.stop_reason
            terminal_evidence = None
            if page_state in {SourcePageState.COMPLETE, SourcePageState.EMPTY}:
                if self._limiter.truncated:
                    page_state = SourcePageState.PARTIAL
                    stop_reason = SourceStopReason.BUDGET_EXHAUSTED
                elif page.terminal_evidence is not None:
                    terminal_evidence = CoverageTerminalEvidence.model_validate(
                        page.terminal_evidence.model_dump()
                    )
            renewed, coverage, progress = CoverageWindowService(
                self._session,
                execution=execution,
                clock=self._clock,
            ).record_cursor_page_in_transaction(
                lease=lease,
                window=window,
                request=request,
                page_state=page_state,
                next_token=page.next_page_token,
                stop_reason=stop_reason,
                evidence=terminal_evidence,
                job_progress=JobProgress(stage=JobStage.SAVE, items_saved=saved_items),
                observed_items=len(page.items),
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
        if comment.root_comment_external_id is not None:
            fields["root_comment_external_id"] = comment.root_comment_external_id
        if comment.reply_target_comment_external_id is not None:
            fields["reply_target_comment_external_id"] = comment.reply_target_comment_external_id
        if comment.parent_relation_status != "unresolved":
            fields["parent_relation_status"] = comment.parent_relation_status
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
