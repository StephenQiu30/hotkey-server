from __future__ import annotations

import hashlib
from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import UTC, datetime
from uuid import UUID, uuid5

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from connections.schemas import SourceEntryPoint
from connections.services import require_source_connection_version
from content.models import ContentDiscovery
from content.schemas import (
    ContentTruncationReason,
    KeywordDiscoveryRunInput,
    PersistContentPostInput,
)
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
    CollectionScanKind,
    CoverageWindowInput,
    CoverageWindowView,
    JobAcceptanceInput,
    JobObservationContext,
    JobStage,
    UsageAttemptInput,
    UsageKind,
    UsageOutcome,
)
from jobs.services import (
    CoverageWindowService,
    ResourceBudgetService,
    load_job_execution_configuration,
)
from monitors.services import MonitorTopicService, evaluate_monitor_rules
from sources.contracts import (
    SourceCapability,
    SourcePage,
    SourcePageState,
    SourcePost,
    SourceSort,
)

_SEARCH_STAGE = "search.request"


class KeywordRequestMeter:
    """Reserve each actual source HTTP attempt before the adapter sends it."""

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
            raise ValueError("search request budget must be between 1 and 100")
        if deadline_at.utcoffset() is None:
            raise ValueError("search deadline must be timezone-aware")
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
        """Return false only for a fenced cancellation or bounded budget refusal."""
        if attempt != self._started + 1:
            raise ValueError("source request attempt is out of sequence")
        if self._started >= self._max_requests or self._clock() >= self._deadline_at:
            return False
        execution = JobExecutionService(
            self._session, lease_seconds=self._lease_seconds, clock=self._clock
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
                        stage=_SEARCH_STAGE,
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
                    self._session, clock=self._clock
                ).require_admission_ready_in_transaction(
                    owner_id=self._owner_id,
                    source_key=self._source_key,
                    capability=SourceCapability.SEARCH,
                    data_class=DataClass.STRUCTURED,
                )
                if request_sequence >= self._max_requests:
                    return False
                attempt_id = resource_attempt_id(
                    operation_id=self._operation_id,
                    component_key=self._component_key,
                    stage=_SEARCH_STAGE,
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
                        stage=_SEARCH_STAGE,
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
            or (
                page.request_count == 0
                and page.state
                in {
                    SourcePageState.MORE,
                    SourcePageState.COMPLETE,
                    SourcePageState.EMPTY,
                    SourcePageState.PARTIAL,
                }
            )
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
                owner_id=owner_id, reservation_id=attempt_id, actual_units=1
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
        """Charge attempts after a transport or persistence error."""
        self._session.rollback()
        with self._session.begin():
            budget = ResourceBudgetService(self._session, clock=self._clock)
            for attempt_id in self._pending:
                budget.settle_budget_reservation_in_transaction(
                    owner_id=self._owner_id, reservation_id=attempt_id, actual_units=1
                )
                budget.finish_attempt_in_transaction(
                    owner_id=self._owner_id,
                    attempt_id=attempt_id,
                    outcome=UsageOutcome.FAILED,
                    finished_at=self._clock(),
                )
        self._pending.clear()


def _target_hash(configuration_ref: str, query: str) -> bytes:
    return hashlib.sha256(f"{configuration_ref}\0{query}".encode()).digest()


def _topic_id_from_configuration_ref(configuration_ref: str) -> UUID:
    prefix = "topic:"
    if not configuration_ref.startswith(prefix):
        raise ValueError("keyword search requires a topic configuration")
    value = configuration_ref.removeprefix(prefix)
    try:
        topic_id = UUID(value)
    except ValueError as error:
        raise ValueError("keyword search requires a valid topic reference") from error
    if str(topic_id) != value:
        raise ValueError("keyword search requires a canonical topic reference")
    return topic_id


def plan_keyword_discovery(run: KeywordDiscoveryRunInput) -> tuple[JobAcceptanceInput, ...]:
    """Freeze explicit source queries; acceptance waits for a registered, authorized handler."""
    observation = JobObservationContext(
        configuration_ref=run.configuration_ref,
        configuration_version=run.configuration_version,
        source_key=run.source_key,
        source_capability=SourceCapability.SEARCH,
    )
    jobs = []
    for sort, max_pages, max_requests in (
        (SourceSort.LATEST, run.latest_max_pages, run.latest_max_requests),
        (SourceSort.TOP, run.top_max_pages, run.top_max_requests),
    ):
        for index, query in enumerate((run.primary_query, *run.upstream_aliases)):
            jobs.append(
                JobAcceptanceInput(
                    operation_id=uuid5(run.run_id, f"{sort.value}:{index}"),
                    kind="keyword.search",
                    observation=observation,
                    scheduled_for_at=run.scheduled_for_at,
                    scope={
                        "run_id": str(run.run_id),
                        "connection_id": str(run.connection_id),
                        "connection_version": run.connection_version,
                        "query": query,
                        "query_role": "primary" if index == 0 else "upstream_alias",
                        "sort_key": sort.value,
                        "target_hash": _target_hash(run.configuration_ref, query).hex(),
                        "rule_version": run.configuration_version,
                        "starts_at": run.starts_at.isoformat(),
                        "ends_at": run.ends_at.isoformat(),
                        "page_size": run.page_size,
                        "max_pages": max_pages,
                        "max_requests": max_requests,
                        "max_seconds": run.max_seconds,
                        "relevance_filter_position": "local",
                        "scan_kind": CollectionScanKind.NEW_SCAN.value,
                        "entry_point": run.entry_point.value,
                    },
                )
            )
    return tuple(jobs)


def plan_scheduled_keyword_discovery(run: KeywordDiscoveryRunInput) -> JobAcceptanceInput:
    """Build the single latest-search job represented by one collection schedule window."""
    if run.entry_point is not SourceEntryPoint.SCHEDULED:
        raise ValueError("scheduled discovery requires the scheduled entry point")
    observation = JobObservationContext(
        configuration_ref=run.configuration_ref,
        configuration_version=run.configuration_version,
        source_key=run.source_key,
        source_capability=SourceCapability.SEARCH,
    )
    query = run.primary_query
    return JobAcceptanceInput(
        operation_id=run.run_id,
        kind="keyword.search",
        observation=observation,
        scheduled_for_at=run.scheduled_for_at,
        scope={
            "run_id": str(run.run_id),
            "connection_id": str(run.connection_id),
            "connection_version": run.connection_version,
            "query": query,
            "query_role": "primary",
            "sort_key": SourceSort.LATEST.value,
            "target_hash": _target_hash(run.configuration_ref, query).hex(),
            "rule_version": run.configuration_version,
            "starts_at": run.starts_at.isoformat(),
            "ends_at": run.ends_at.isoformat(),
            "page_size": run.page_size,
            "max_pages": run.latest_max_pages,
            "max_requests": run.latest_max_requests,
            "max_seconds": run.max_seconds,
            "relevance_filter_position": "local",
            "scan_kind": CollectionScanKind.NEW_SCAN.value,
            "entry_point": run.entry_point.value,
        },
    )


@dataclass(frozen=True, slots=True)
class KeywordPageCommitResult:
    lease: ExecutionLease
    coverage: CoverageWindowView
    saved_items: int
    filtered_items: int
    progress: CursorPageProgress = field(repr=False)


class KeywordDiscoveryPageCommitService:
    """Commit one admitted social search page and its cursor in one transaction."""

    def __init__(
        self,
        session: Session,
        *,
        lease_seconds: int,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._session = session
        self._lease_seconds = lease_seconds
        self._clock = clock or (lambda: datetime.now(UTC))

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
        meter: KeywordRequestMeter | None = None,
    ) -> KeywordPageCommitResult:
        configuration = load_job_execution_configuration(self._session, job_id=lease.job_id)
        if (
            configuration is None
            or configuration.owner_id != owner_id
            or configuration.kind != "keyword.search"
            or configuration.observation.source_key != window.source_key
            or configuration.observation.source_capability is not SourceCapability.SEARCH
            or configuration.observation.configuration_version != window.rule_version
            or configuration.scope.get("connection_id") != str(connection_id)
            or configuration.scope.get("connection_version") != connection_version
            or configuration.scope.get("target_hash") != window.target_hash.hex()
            or configuration.scope.get("rule_version") != window.rule_version
            or configuration.scope.get("sort_key") != window.sort_key.value
            or configuration.scope.get("starts_at") != window.starts_at.isoformat()
            or configuration.scope.get("ends_at") != window.ends_at.isoformat()
            or configuration.scope.get("max_pages") != request.max_pages
            or configuration.scope.get("relevance_filter_position") != "local"
            or window.owner_id != owner_id
            or window.capability is not SourceCapability.SEARCH
            or page.source_key != window.source_key
            or page.capability is not SourceCapability.SEARCH
        ):
            self._session.rollback()
            raise ValueError("keyword page does not match the accepted search scope")
        query = configuration.scope.get("query")
        entry_point_value = configuration.scope.get("entry_point")
        if (
            not isinstance(query, str)
            or _target_hash(configuration.observation.configuration_ref, query)
            != window.target_hash
            or not isinstance(entry_point_value, str)
        ):
            self._session.rollback()
            raise ValueError("keyword query hash does not match the accepted scope")
        try:
            entry_point = SourceEntryPoint(entry_point_value)
        except ValueError:
            self._session.rollback()
            raise ValueError("keyword entry point is invalid") from None
        try:
            topic_id = _topic_id_from_configuration_ref(configuration.observation.configuration_ref)
        except ValueError:
            self._session.rollback()
            raise

        policy = SourceAccessPolicyService(self._session, clock=self._clock)
        candidates: list[SourcePost] = []
        seen: set[str] = set()
        filtered_items = 0
        for item in page.items:
            if not isinstance(item, SourcePost):
                self._session.rollback()
                raise ValueError("keyword search page can contain only posts")
            if (
                item.published_at is not None
                and not window.starts_at <= item.published_at < window.ends_at
            ) or item.external_id in seen:
                filtered_items += 1
                continue
            seen.add(item.external_id)
            candidates.append(item)

        self._session.rollback()
        execution = JobExecutionService(
            self._session, lease_seconds=self._lease_seconds, clock=self._clock
        )
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
                raise CheckpointConflictError("keyword page belongs to an older checkpoint")
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
                capability=SourceCapability.SEARCH,
                data_class=DataClass.STRUCTURED,
            )
            topic_rules = MonitorTopicService(self._session).get_topic_rules_in_transaction(
                owner_id=owner_id,
                topic_id=topic_id,
                version=window.rule_version,
            )
            content = ContentService(self._session, clock=self._clock)
            for item in candidates:
                matched_text = "\n".join(part for part in (item.title, item.text) if part)
                if (
                    not matched_text
                    or not evaluate_monitor_rules(topic_rules, matched_text).matched
                ):
                    filtered_items += 1
                    continue
                admission = policy.admit_payload_in_transaction(
                    owner_id=owner_id,
                    source_key=window.source_key,
                    capability=SourceCapability.SEARCH,
                    data_class=DataClass.STRUCTURED,
                    collected_at=page.observed_at,
                    payload=self._payload(item),
                )
                content.persist_post_in_transaction(
                    owner_id=owner_id,
                    command=PersistContentPostInput(
                        job_id=lease.job_id,
                        source_operation_id=uuid5(page_operation_id, item.external_id),
                        connection_id=connection_id,
                        connection_version=connection_version,
                        entry_point=entry_point,
                        component_name=f"{window.source_key}.search",
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
            renewed, coverage, progress = CoverageWindowService(
                self._session, execution=execution, clock=self._clock
            ).record_cursor_page_in_transaction(
                lease=lease,
                window=window,
                request=request,
                page_state=page.state,
                next_token=page.next_page_token,
                stop_reason=page.stop_reason,
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
        return KeywordPageCommitResult(
            lease=renewed,
            coverage=coverage,
            saved_items=saved_items,
            filtered_items=filtered_items,
            progress=progress,
        )

    @staticmethod
    def _payload(post: SourcePost) -> dict[str, object]:
        fields: dict[str, object] = {
            "object_type": "post",
            "external_id": post.external_id,
            "published_at": post.published_at.isoformat() if post.published_at else None,
            "like_count": post.like_count,
            "comment_count": post.comment_count,
            "repost_count": post.repost_count,
            "view_count": post.view_count,
            "play_count": post.play_count,
            "danmaku_count": post.danmaku_count,
        }
        if post.canonical_url is not None:
            fields["canonical_url"] = post.canonical_url
        if post.author_external_id is not None:
            fields["author_external_id"] = post.author_external_id
        if post.author_name is not None:
            fields["author_name"] = post.author_name
        if post.title is not None:
            fields["title"] = post.title
        if post.text is not None or post.title is not None:
            if post.text is not None:
                fields["body"] = post.text
            fields["text_scope"] = post.text_scope or "full"
            fields["text_origin"] = "source"
            if post.text_scope == "truncated":
                fields["truncation_reason"] = ContentTruncationReason.SOURCE_LIMIT.value
        elif post.text_scope is not None:
            fields["text_scope"] = post.text_scope
            fields["text_origin"] = "source"
        if post.quote_external_id is not None:
            fields["quote_target_external_id"] = post.quote_external_id
        if post.repost_external_id is not None:
            fields["repost_target_external_id"] = post.repost_external_id
        return fields
