from __future__ import annotations

import hashlib
from collections.abc import Callable
from datetime import UTC, datetime
from uuid import UUID, uuid5

from sqlalchemy.orm import Session, sessionmaker

from content.discovery import KeywordDiscoveryPageCommitService, KeywordRequestMeter
from jobs.cursor import CursorBudgetExhaustedError, plan_cursor_request
from jobs.execution import ExecutionLease, JobCompletion, JobExecutionFailure, JobExecutionService
from jobs.schemas import CoverageWindowInput, JobFailureCategory, JobMessage, JobStatus
from jobs.services import JobExecutionConfiguration, load_job_execution_configuration
from sources.contracts import (
    SearchRequest,
    SourceAdapter,
    SourceCapability,
    SourcePageState,
    SourceSort,
    SourceStopReason,
)

type SearchAdapterFactory = Callable[
    [Callable[[int], bool], Callable[[], bool], int], SourceAdapter
]


class KeywordDiscoveryExecutor:
    """Run an accepted search against a caller-supplied, bounded source adapter."""

    def __init__(
        self,
        sessions: sessionmaker[Session],
        *,
        lease_seconds: int,
        component_key: str,
        adapter_factory: SearchAdapterFactory,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._sessions = sessions
        self._lease_seconds = lease_seconds
        self._component_key = component_key
        self._adapter_factory = adapter_factory
        self._clock = clock or (lambda: datetime.now(UTC))

    def execute(
        self, message: JobMessage, lease: ExecutionLease
    ) -> tuple[ExecutionLease, JobCompletion]:
        configuration = self._configuration(message)
        scope = configuration.scope
        try:
            source_key = configuration.observation.source_key
            if source_key is None:
                raise ValueError("search source is missing")
            if set(scope) != {
                "run_id",
                "connection_id",
                "connection_version",
                "query",
                "query_role",
                "sort_key",
                "target_hash",
                "rule_version",
                "starts_at",
                "ends_at",
                "page_size",
                "max_pages",
                "max_requests",
                "scan_kind",
            }:
                raise ValueError("search scope fields are incomplete")
            UUID(self._required_str(scope, "run_id"))
            connection_id = UUID(self._required_str(scope, "connection_id"))
            connection_version = self._required_int(scope, "connection_version")
            max_pages = self._required_int(scope, "max_pages")
            max_requests = self._required_int(scope, "max_requests")
            page_size = self._required_int(scope, "page_size")
            query = self._required_str(scope, "query")
            query_role = self._required_str(scope, "query_role")
            sort = SourceSort(self._required_str(scope, "sort_key"))
            target_hash = bytes.fromhex(self._required_str(scope, "target_hash"))
            window = CoverageWindowInput(
                owner_id=configuration.owner_id,
                source_key=source_key,
                capability=SourceCapability.SEARCH,
                target_hash=target_hash,
                sort_key=sort,
                rule_version=self._required_int(scope, "rule_version"),
                starts_at=datetime.fromisoformat(self._required_str(scope, "starts_at")),
                ends_at=datetime.fromisoformat(self._required_str(scope, "ends_at")),
            )
            if (
                not 1 <= max_pages <= 20
                or not 1 <= max_requests <= 100
                or connection_version < 1
                or query_role not in {"primary", "upstream_alias"}
                or window.rule_version != configuration.observation.configuration_version
                or target_hash
                != hashlib.sha256(
                    f"{configuration.observation.configuration_ref}\0{query}".encode()
                ).digest()
                or scope["scan_kind"] != "new_scan"
            ):
                raise ValueError("search scope limits are invalid")
            SearchRequest(
                source_key=window.source_key,
                query=query,
                sort=sort,
                page_size=page_size,
            )
        except (TypeError, ValueError) as error:
            raise self._failure(
                "search_scope_invalid",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "重新提交有界的关键词搜索任务",
            ) from error

        with self._sessions() as session:
            meter = KeywordRequestMeter(
                session,
                owner_id=configuration.owner_id,
                lease=lease,
                operation_id=configuration.operation_id,
                source_key=window.source_key,
                connection_id=connection_id,
                connection_version=connection_version,
                component_key=self._component_key,
                max_requests=max_requests,
                lease_seconds=self._lease_seconds,
                clock=self._clock,
            )
            execution = JobExecutionService(
                session, lease_seconds=self._lease_seconds, clock=self._clock
            )
            adapter = self._adapter_factory(
                meter.before_request,
                lambda: execution.cancellation_requested(lease),
                max_requests,
            )
            if (
                adapter.source_key != window.source_key
                or SourceCapability.SEARCH not in adapter.capabilities
            ):
                raise self._failure(
                    "search_adapter_unavailable",
                    JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                    "配置与任务来源一致的关键词搜索适配器",
                )
            live_token: str | None = None
            while True:
                try:
                    cursor = plan_cursor_request(
                        window=window,
                        checkpoint=lease.checkpoint,
                        live_token=live_token,
                        max_pages=max_pages,
                        max_rescans=1,
                    )
                except CursorBudgetExhaustedError:
                    return lease, self._partial("budget_exhausted")
                request = SearchRequest(
                    source_key=window.source_key,
                    query=query,
                    sort=sort,
                    page_size=page_size,
                    page_token=cursor.token,
                )
                try:
                    page = adapter.fetch_page(request)
                    result = KeywordDiscoveryPageCommitService(
                        session, lease_seconds=self._lease_seconds, clock=self._clock
                    ).commit_page(
                        owner_id=configuration.owner_id,
                        lease=lease,
                        window=window,
                        request=cursor,
                        page_operation_id=uuid5(
                            configuration.operation_id,
                            f"page:{cursor.pages}:{cursor.rescans}",
                        ),
                        connection_id=connection_id,
                        connection_version=connection_version,
                        page=page,
                        meter=meter,
                    )
                except Exception:
                    meter.fail_pending()
                    raise
                lease = result.lease
                if result.progress.state is SourcePageState.MORE:
                    live_token = result.progress.next_token
                    continue
                if result.coverage.status == "confirmed":
                    return lease, JobCompletion(status=JobStatus.SUCCEEDED)
                reason = result.coverage.stop_reason or "unverified_terminal"
                return lease, self._partial(reason)

    def _configuration(self, message: JobMessage) -> JobExecutionConfiguration:
        with self._sessions() as session:
            configuration = load_job_execution_configuration(session, job_id=message.job_id)
        if (
            configuration is None
            or configuration.owner_id != message.owner_id
            or configuration.operation_id != message.operation_id
            or configuration.kind != "keyword.search"
            or message.kind != configuration.kind
            or message.configuration_ref != configuration.observation.configuration_ref
            or message.configuration_version != configuration.observation.configuration_version
            or message.source_key != configuration.observation.source_key
            or message.source_capability is not SourceCapability.SEARCH
            or configuration.observation.source_capability is not SourceCapability.SEARCH
        ):
            raise self._failure(
                "search_context_invalid",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "重新提交包含当前来源和查询版本的关键词搜索任务",
            )
        return configuration

    @staticmethod
    def _required_str(scope: dict[str, str | int | bool | None], key: str) -> str:
        value = scope.get(key)
        if not isinstance(value, str):
            raise ValueError(f"search scope {key} must be a string")
        return value

    @staticmethod
    def _required_int(scope: dict[str, str | int | bool | None], key: str) -> int:
        value = scope.get(key)
        if type(value) is not int:
            raise ValueError(f"search scope {key} must be an integer")
        return value

    def _partial(self, reason: str) -> JobCompletion:
        category = (
            JobFailureCategory.RATE_LIMITED
            if reason == SourceStopReason.RATE_LIMITED.value
            else JobFailureCategory.CONFIGURATION_UNAVAILABLE
            if reason in {"unverified_terminal", SourceStopReason.UNSUPPORTED.value}
            else JobFailureCategory.TRANSIENT
        )
        return JobCompletion(
            status=JobStatus.PARTIALLY_SUCCEEDED,
            failure=self._failure(
                "search_scope_incomplete",
                category,
                f"检查来源范围或停止原因: {reason}",
            ),
        )

    def _failure(
        self, code: str, category: JobFailureCategory, next_action: str
    ) -> JobExecutionFailure:
        return JobExecutionFailure(
            error_code=code,
            category=category,
            occurred_at=self._clock(),
            next_action=next_action,
            manual_retry_allowed=True,
        )
