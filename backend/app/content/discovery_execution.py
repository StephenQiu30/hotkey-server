from __future__ import annotations

import hashlib
from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid5

from sqlalchemy.orm import Session, sessionmaker

from content.discovery import KeywordDiscoveryPageCommitService, KeywordRequestMeter
from core.errors import ApplicationError
from evidence.services import RetentionPolicyUnavailableError, SourceAccessUnavailableError
from jobs.cursor import CursorBudgetExhaustedError, plan_cursor_request
from jobs.execution import (
    ExecutionLease,
    JobCompletion,
    JobExecutionFailure,
    JobExecutionService,
    JobLeaseUnavailableError,
    StaleExecutionLeaseError,
)
from jobs.schemas import CoverageWindowInput, JobFailureCategory, JobMessage, JobStatus
from jobs.services import (
    BudgetPolicyUnavailableError,
    ComponentPolicyUnavailableError,
    CoverageWindowService,
    JobExecutionConfiguration,
    load_job_execution_configuration,
)
from sources.contracts import (
    SearchRequest,
    SourceAdapter,
    SourceCapability,
    SourcePageState,
    SourceSort,
    SourceStopReason,
)

type SearchAdapterFactory = Callable[
    [Callable[[int], bool], Callable[[], bool], int, float], SourceAdapter
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
                "max_seconds",
                "scan_kind",
            }:
                raise ValueError("search scope fields are incomplete")
            UUID(self._required_str(scope, "run_id"))
            connection_id = UUID(self._required_str(scope, "connection_id"))
            connection_version = self._required_int(scope, "connection_version")
            max_pages = self._required_int(scope, "max_pages")
            max_requests = self._required_int(scope, "max_requests")
            max_seconds = self._required_int(scope, "max_seconds")
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
                or not 1 <= max_seconds <= 90
                or connection_version < 1
                or configuration.started_at is None
                or configuration.started_at.utcoffset() is None
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
                starts_at=window.starts_at,
                ends_at=window.ends_at,
            )
        except (TypeError, ValueError) as error:
            raise self._failure(
                "search_scope_invalid",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "重新提交有界的关键词搜索任务",
            ) from error

        assert configuration.started_at is not None
        deadline_at = configuration.started_at + timedelta(seconds=max_seconds)
        with self._sessions() as session:
            execution = JobExecutionService(
                session, lease_seconds=self._lease_seconds, clock=self._clock
            )
            if self._stop_if_cancelled(session, execution=execution, lease=lease, window=window):
                return lease, JobCompletion(status=JobStatus.SUCCEEDED)
            if self._clock() >= deadline_at:
                self._mark_local_stop(
                    session, lease=lease, window=window, reason=SourceStopReason.BUDGET_EXHAUSTED
                )
                return lease, self._partial(
                    SourceStopReason.BUDGET_EXHAUSTED.value,
                    code="search_time_budget_exhausted",
                )
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
                deadline_at=deadline_at,
                lease_seconds=self._lease_seconds,
                clock=self._clock,
            )
            adapter = self._adapter_factory(
                meter.before_request,
                lambda: execution.cancellation_requested(lease),
                max_requests,
                max(0.001, (deadline_at - self._clock()).total_seconds()),
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
                if self._stop_if_cancelled(
                    session, execution=execution, lease=lease, window=window
                ):
                    return lease, JobCompletion(status=JobStatus.SUCCEEDED)
                if self._clock() >= deadline_at:
                    self._mark_local_stop(
                        session,
                        lease=lease,
                        window=window,
                        reason=SourceStopReason.BUDGET_EXHAUSTED,
                    )
                    return lease, self._partial(
                        SourceStopReason.BUDGET_EXHAUSTED.value,
                        code="search_time_budget_exhausted",
                    )
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
                    starts_at=window.starts_at,
                    ends_at=window.ends_at,
                )
                try:
                    page = adapter.fetch_page(request)
                except JobLeaseUnavailableError:
                    meter.fail_pending()
                    if not self._stop_if_cancelled(
                        session, execution=execution, lease=lease, window=window
                    ):
                        raise
                    return lease, JobCompletion(status=JobStatus.SUCCEEDED)
                except StaleExecutionLeaseError:
                    meter.fail_pending()
                    raise
                except Exception as error:
                    meter.fail_pending()
                    if self._stop_if_cancelled(
                        session, execution=execution, lease=lease, window=window
                    ):
                        return lease, JobCompletion(status=JobStatus.SUCCEEDED)
                    failure = self._adapter_failure(error)
                    if failure.category in {
                        JobFailureCategory.TRANSIENT,
                        JobFailureCategory.INVALID_RESPONSE,
                    }:
                        self._mark_local_stop(
                            session,
                            lease=lease,
                            window=window,
                            reason=(
                                SourceStopReason.UPSTREAM_ERROR
                                if failure.category is JobFailureCategory.TRANSIENT
                                else SourceStopReason.PROTOCOL_ERROR
                            ),
                        )
                    raise failure from error
                try:
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
                except JobLeaseUnavailableError:
                    meter.fail_pending()
                    if not self._stop_if_cancelled(
                        session, execution=execution, lease=lease, window=window
                    ):
                        raise
                    return lease, JobCompletion(status=JobStatus.SUCCEEDED)
                except (
                    ApplicationError,
                    SourceAccessUnavailableError,
                    RetentionPolicyUnavailableError,
                ) as error:
                    meter.fail_pending()
                    raise self._failure(
                        "search_source_policy_changed",
                        JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                        "恢复当前连接和来源保留政策后重试",
                    ) from error
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
                if (
                    result.progress.state is SourcePageState.STOPPED
                    and result.saved_items == 0
                    and page.stop_reason is not SourceStopReason.BUDGET_EXHAUSTED
                ):
                    can_retry = False
                    if page.stop_reason in {
                        SourceStopReason.RATE_LIMITED,
                        SourceStopReason.UPSTREAM_ERROR,
                    }:
                        session.rollback()
                        try:
                            with session.begin():
                                requests_sent = execution.current_request_count_in_transaction(
                                    lease,
                                    owner_id=configuration.owner_id,
                                    operation_id=configuration.operation_id,
                                )
                        except JobLeaseUnavailableError:
                            if not self._stop_if_cancelled(
                                session, execution=execution, lease=lease, window=window
                            ):
                                raise
                            return lease, JobCompletion(status=JobStatus.SUCCEEDED)
                        pages = result.progress.checkpoint.get("cursor.pages")
                        rescans = result.progress.checkpoint.get("cursor.rescans")
                        can_retry = (
                            result.progress.checkpoint.get("cursor.restart") is True
                            and isinstance(pages, int)
                            and not isinstance(pages, bool)
                            and pages < max_pages
                            and isinstance(rescans, int)
                            and not isinstance(rescans, bool)
                            and rescans < 1
                            and requests_sent < max_requests
                        )
                    raise self._source_failure(
                        page.stop_reason,
                        retry_at=page.retry_at,
                        deadline_at=deadline_at,
                        can_retry=can_retry,
                    )
                return lease, self._partial(reason)

    def _stop_if_cancelled(
        self,
        session: Session,
        *,
        execution: JobExecutionService,
        lease: ExecutionLease,
        window: CoverageWindowInput,
    ) -> bool:
        if not execution.cancellation_requested(lease):
            return False
        self._mark_local_stop(
            session, lease=lease, window=window, reason=SourceStopReason.CANCELLED
        )
        return True

    def _mark_local_stop(
        self,
        session: Session,
        *,
        lease: ExecutionLease,
        window: CoverageWindowInput,
        reason: SourceStopReason,
    ) -> None:
        session.rollback()
        with session.begin():
            CoverageWindowService(
                session,
                execution=JobExecutionService(
                    session, lease_seconds=self._lease_seconds, clock=self._clock
                ),
                clock=self._clock,
            ).mark_partial_without_page_in_transaction(
                lease=lease,
                window=window,
                stop_reason=reason,
                allow_cancel=reason is SourceStopReason.CANCELLED,
            )

    def _adapter_failure(self, error: Exception) -> JobExecutionFailure:
        if isinstance(error, ApplicationError):
            return self._failure(
                "search_connection_changed",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "使用当前连接版本重新提交搜索任务",
            )
        if isinstance(
            error,
            (
                SourceAccessUnavailableError,
                RetentionPolicyUnavailableError,
                ComponentPolicyUnavailableError,
                BudgetPolicyUnavailableError,
            ),
        ):
            return self._failure(
                "search_policy_unavailable",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "配置可用的来源、组件与预算政策后重试",
            )
        if isinstance(error, ValueError):
            return self._failure(
                "search_source_invalid_response",
                JobFailureCategory.INVALID_RESPONSE,
                "检查来源响应后手动重试",
            )
        return self._failure(
            "search_source_failed",
            JobFailureCategory.TRANSIENT,
            "检查来源状态后手动重试",
        )

    def _source_failure(
        self,
        reason: SourceStopReason | None,
        *,
        retry_at: datetime | None,
        deadline_at: datetime,
        can_retry: bool,
    ) -> JobExecutionFailure:
        if reason is SourceStopReason.AUTHENTICATION_REQUIRED:
            return self._failure(
                "source_authentication_required",
                JobFailureCategory.AUTHENTICATION_REQUIRED,
                "完成来源认证后重新提交搜索任务",
                manual_retry_allowed=False,
            )
        if reason is SourceStopReason.ACCESS_DENIED:
            return self._failure(
                "source_access_denied",
                JobFailureCategory.PERMISSION_DENIED,
                "检查来源权限后重新提交搜索任务",
                manual_retry_allowed=False,
            )
        if reason is SourceStopReason.RATE_LIMITED:
            now = self._clock()
            candidate = (
                max(now + timedelta(seconds=30), retry_at)
                if retry_at
                else now + timedelta(seconds=30)
            )
            if not can_retry or candidate >= deadline_at:
                return self._failure(
                    "source_rate_limited",
                    JobFailureCategory.RATE_LIMITED,
                    "等待来源限流恢复后重新提交搜索任务",
                    manual_retry_allowed=False,
                )
            return self._failure(
                "source_rate_limited",
                JobFailureCategory.RATE_LIMITED,
                "等待来源限流恢复后自动重试",
                retry_at=candidate,
                max_attempts=3,
            )
        if reason is SourceStopReason.UPSTREAM_ERROR:
            candidate = self._clock() + timedelta(seconds=30)
            if not can_retry or candidate >= deadline_at:
                return self._failure(
                    "source_upstream_unavailable",
                    JobFailureCategory.TRANSIENT,
                    "等待来源恢复后重新提交搜索任务",
                    manual_retry_allowed=False,
                )
            return self._failure(
                "source_upstream_unavailable",
                JobFailureCategory.TRANSIENT,
                "等待来源恢复后自动重试",
                retry_at=candidate,
                max_attempts=3,
            )
        if reason is SourceStopReason.PROTOCOL_ERROR:
            return self._failure(
                "search_source_invalid_response",
                JobFailureCategory.INVALID_RESPONSE,
                "检查来源响应后重新提交搜索任务",
                manual_retry_allowed=False,
            )
        if reason is SourceStopReason.NOT_FOUND:
            return self._failure(
                "search_source_not_found",
                JobFailureCategory.INVALID_INPUT,
                "检查关键词与来源后重新提交",
                manual_retry_allowed=False,
            )
        if reason is SourceStopReason.UNSUPPORTED:
            return self._failure(
                "search_source_unsupported",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "选择支持当前搜索条件的来源",
                manual_retry_allowed=False,
            )
        return self._failure(
            "search_source_stopped",
            JobFailureCategory.TRANSIENT,
            "检查来源停止原因后重新提交搜索任务",
            manual_retry_allowed=False,
        )

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

    def _partial(self, reason: str, *, code: str = "search_scope_incomplete") -> JobCompletion:
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
                code,
                category,
                f"检查来源范围或停止原因: {reason}",
                manual_retry_allowed=False,
            ),
        )

    def _failure(
        self,
        code: str,
        category: JobFailureCategory,
        next_action: str,
        *,
        manual_retry_allowed: bool = True,
        retry_at: datetime | None = None,
        max_attempts: int | None = None,
    ) -> JobExecutionFailure:
        return JobExecutionFailure(
            error_code=code,
            category=category,
            occurred_at=self._clock(),
            next_action=next_action,
            manual_retry_allowed=manual_retry_allowed,
            retry_at=retry_at,
            max_attempts=max_attempts,
        )
