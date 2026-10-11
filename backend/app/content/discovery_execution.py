from __future__ import annotations

import hashlib
from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid5

from sqlalchemy.orm import Session, sessionmaker

from connections.schemas import SourceConnectionConfig
from connections.services import (
    pause_bilibili_connection_in_transaction,
    require_source_connection_version,
)
from content.discovery import KeywordDiscoveryPageCommitService, KeywordRequestMeter
from core.config import get_settings
from core.errors import ApplicationError
from events.heat import record_source_fetch_success_in_transaction
from evidence.services import RetentionPolicyUnavailableError, SourceAccessUnavailableError
from jobs.collection_topics import TOPIC_COLLECTION_SEQUENCE
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
from sources.adapters.bilibili_chrome import VERSION as CHROME_VERSION
from sources.adapters.bilibili_chrome import BilibiliChromeAdapter
from sources.adapters.hackernews import HackerNewsAdapter
from sources.adapters.mediacrawler import (
    MediaCrawlerExecutionError,
    MediaCrawlerPreflightError,
    validate_job_version_evidence,
)
from sources.adapters.rss import GOOGLE_NEWS_FEED_URL_TEMPLATE, RssSourceAdapter
from sources.adapters.rsshub_endpoint import is_fixed_rsshub_endpoint
from sources.adapters.web_search import WebSearchAdapter
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


class SearchRequestGuardUnavailableError(ValueError):
    """A subprocess bridge cannot guard each outbound request before sending."""


class UnsupportedSearchSourceError(ValueError):
    """The accepted source key has no registered keyword-search adapter."""


def build_search_adapter_factory(
    source_key: str,
    config: SourceConnectionConfig,
    *,
    owner_id: UUID | None = None,
) -> SearchAdapterFactory:
    allowed_hosts = frozenset(config.allowed_hosts)
    if not allowed_hosts:
        raise ValueError("search adapter requires allowed_hosts")
    if source_key == "bilibili":
        settings = get_settings()
        if config.bilibili_transport == "chrome":
            if owner_id is None or settings.bilibili_chrome_owner_id != owner_id:
                raise ValueError("Chrome requires the configured local account")
            return lambda before_request, cancelled, max_requests, max_seconds: (
                BilibiliChromeAdapter(
                    identity_env=settings.bilibili_chrome_identity_env,
                    before_request=before_request,
                    cancelled=cancelled,
                    max_requests=max_requests,
                    max_seconds=max_seconds,
                )
            )
        if not settings.mediacrawler_enabled or owner_id is None:
            raise ValueError("MediaCrawler requires enabled host configuration and owner")
        # A prepaid request upper bound plus process polling cannot enforce a
        # topic pause between the crawler's individual HTTP calls.
        raise SearchRequestGuardUnavailableError("mediacrawler_request_guard_unavailable")
    if source_key == "hackernews":
        if (
            config.base_url is None
            or str(config.base_url).rstrip("/") != "https://hn.algolia.com/api/v1"
            or allowed_hosts != frozenset({"hn.algolia.com"})
        ):
            raise ValueError("Hacker News requires the fixed HTTPS Algolia endpoint")
        base_url = str(config.base_url)
        return lambda before_request, cancelled, max_requests, max_seconds: HackerNewsAdapter(
            base_url=base_url,
            allowed_hosts=allowed_hosts,
            before_request=before_request,
            cancelled=cancelled,
            max_requests=max_requests,
            max_seconds=max_seconds,
        )
    if source_key == "google_news" or source_key.startswith("rss_"):
        if config.feed_url_template is None:
            raise ValueError("RSS adapter requires feed_url_template")
        feed_url_template = config.feed_url_template
        if source_key == "rss_36kr" and not is_fixed_rsshub_endpoint(
            feed_url_template,
            route="/36kr/newsflashes",
            allowed_hosts=allowed_hosts,
        ):
            raise ValueError("rss_36kr requires the local RSSHub newsflashes endpoint")
        if source_key == "google_news" and (
            feed_url_template != GOOGLE_NEWS_FEED_URL_TEMPLATE
            or allowed_hosts != frozenset({"news.google.com"})
        ):
            raise ValueError("Google News requires the fixed HTTPS search RSS endpoint")
        return lambda before_request, cancelled, max_requests, max_seconds: RssSourceAdapter(
            source_key=source_key,
            feed_url_template=feed_url_template,
            allowed_hosts=allowed_hosts,
            before_request=before_request,
            cancelled=cancelled,
            max_requests=max_requests,
            max_seconds=max_seconds,
        )
    if source_key == "news_search":
        if config.base_url is None:
            raise ValueError("web search adapter requires base_url")
        base_url = str(config.base_url)
        engines = ",".join(config.engines)
        return lambda before_request, cancelled, max_requests, max_seconds: WebSearchAdapter(
            source_key=source_key,
            base_url=base_url,
            engines=engines,
            allowed_hosts=allowed_hosts,
            before_request=before_request,
            cancelled=cancelled,
            max_requests=max_requests,
            max_seconds=max_seconds,
        )
    raise UnsupportedSearchSourceError(source_key)


class KeywordDiscoveryExecutor:
    """Run an accepted search against a caller-supplied, bounded source adapter."""

    def __init__(
        self,
        sessions: sessionmaker[Session],
        *,
        lease_seconds: int,
        component_key: str | None = None,
        adapter_factory: SearchAdapterFactory | None = None,
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
        scope = {
            key: value
            for key, value in configuration.scope.items()
            if key not in {"source_adapter_version", TOPIC_COLLECTION_SEQUENCE}
        }
        try:
            source_key = configuration.observation.source_key
            if source_key is None:
                raise ValueError("search source is missing")
            search_scope_fields = {
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
                "relevance_filter_position",
                "scan_kind",
                "entry_point",
            }
            manual_scope_fields = {
                "manual_request_id",
                "manual_source_keys",
                "manual_query_index",
                "manual_skips",
            }
            scheduled_scope_fields = {"schedule_key", "due_at"}
            if set(scope) not in (
                search_scope_fields,
                search_scope_fields | manual_scope_fields,
                search_scope_fields | scheduled_scope_fields,
            ):
                raise ValueError("search scope fields are incomplete")
            if manual_scope_fields.issubset(scope):
                UUID(self._required_str(scope, "manual_request_id"))
                self._required_str(scope, "manual_source_keys")
                if (
                    self._required_int(scope, "manual_query_index") < 0
                    or not isinstance(scope["manual_skips"], str)
                    or scope["entry_point"] != "manual"
                ):
                    raise ValueError("manual search scope fields are invalid")
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
            if scheduled_scope_fields.issubset(scope):
                schedule_key = self._required_str(scope, "schedule_key")
                due_at = datetime.fromisoformat(self._required_str(scope, "due_at"))
                if (
                    str(UUID(schedule_key)) != schedule_key
                    or scope["entry_point"] != "scheduled"
                    or due_at.utcoffset() != timedelta(0)
                    or due_at != window.ends_at
                ):
                    raise ValueError("scheduled search scope fields are invalid")
            if (
                not 1 <= max_pages <= 20
                or not 1 <= max_requests <= 100
                or not 1 <= max_seconds <= (220 if source_key == "bilibili" else 90)
                or (source_key == "bilibili" and (page_size > 5 or max_pages != 1))
                or scope["relevance_filter_position"] != "local"
                or connection_version < 1
                or configuration.started_at is None
                or configuration.started_at.utcoffset() is None
                or query_role not in {"primary", "upstream_alias"}
                or scope["entry_point"] not in {"manual", "scheduled"}
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

        if source_key == "bilibili":
            try:
                if configuration.adapter_version != CHROME_VERSION:
                    validate_job_version_evidence(
                        upstream_revision=configuration.upstream_revision,
                        patched_revision=configuration.patched_revision,
                        adapter_version=configuration.adapter_version,
                    )
            except MediaCrawlerPreflightError as error:
                raise self._adapter_failure(error) from error
            try:
                with self._sessions() as session, session.begin():
                    accepted_config = require_source_connection_version(
                        session,
                        owner_id=configuration.owner_id,
                        source_key=source_key,
                        connection_id=connection_id,
                        connection_version=connection_version,
                    )
            except ApplicationError as error:
                raise self._adapter_failure(error) from error
            if (accepted_config.bilibili_transport == "chrome") != (
                configuration.adapter_version == CHROME_VERSION
            ):
                raise self._failure(
                    "source_adapter_version_mismatch",
                    JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                    "重新应用来源预设并提交任务",
                )

        if lease.checkpoint.get("cursor.done") is True:
            with self._sessions() as session, session.begin():
                if CoverageWindowService(
                    session,
                    execution=JobExecutionService(
                        session, lease_seconds=self._lease_seconds, clock=self._clock
                    ),
                    clock=self._clock,
                ).confirmed_by_job_in_transaction(lease=lease, window=window):
                    return lease, JobCompletion(status=JobStatus.SUCCEEDED)

        try:
            adapter_factory = self._adapter_factory or self._configured_adapter_factory(
                owner_id=configuration.owner_id,
                source_key=source_key,
                connection_id=connection_id,
                connection_version=connection_version,
            )
        except SearchRequestGuardUnavailableError as error:
            raise self._failure(
                "search_request_guard_unavailable",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "补齐MediaCrawler搜索的逐请求准入后重新提交或改用Chrome方式",
                manual_retry_allowed=False,
            ) from error
        except UnsupportedSearchSourceError as error:
            raise self._failure(
                "search_source_key_unsupported",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "选择已注册关键词搜索适配器的来源",
                manual_retry_allowed=False,
            ) from error
        except (ApplicationError, RuntimeError, ValueError) as error:
            raise self._failure(
                "search_adapter_config_invalid",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "修复当前连接版本的关键词搜索配置后重新提交",
                manual_retry_allowed=False,
            ) from error

        assert configuration.started_at is not None
        if configuration.collection_cycle_started_at is None:
            raise self._failure(
                "search_cycle_unavailable",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "检查任务采集预算周期后重新提交",
            )
        deadline_at = configuration.collection_cycle_started_at + timedelta(seconds=max_seconds)
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
                component_key=self._component_key or f"collector.{source_key}",
                max_requests=max_requests,
                deadline_at=deadline_at,
                lease_seconds=self._lease_seconds,
                clock=self._clock,
            )
            adapter = adapter_factory(
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
                    session.rollback()
                    with session.begin():
                        execution.require_current_lease_in_transaction(lease)
                        require_source_connection_version(
                            session,
                            owner_id=configuration.owner_id,
                            source_key=source_key,
                            connection_id=connection_id,
                            connection_version=connection_version,
                        )
                        record_source_fetch_success_in_transaction(
                            session,
                            owner_id=configuration.owner_id,
                            source_key=source_key,
                            selector_kind="native_scope",
                            selector_ref=f"search:{window.target_hash.hex()}",
                            completed_at=self._clock(),
                        )
                    return lease, JobCompletion(status=JobStatus.SUCCEEDED)
                reason = result.coverage.stop_reason or "unverified_terminal"
                if (
                    result.progress.state is SourcePageState.STOPPED
                    and result.saved_items == 0
                    and page.stop_reason is not SourceStopReason.BUDGET_EXHAUSTED
                ):
                    if source_key == "bilibili" and page.stop_reason in {
                        SourceStopReason.AUTHENTICATION_REQUIRED,
                        SourceStopReason.RATE_LIMITED,
                    }:
                        session.rollback()
                        with session.begin():
                            pause_bilibili_connection_in_transaction(
                                session,
                                owner_id=configuration.owner_id,
                                connection_id=connection_id,
                                connection_version=connection_version,
                                now=self._clock(),
                                reason=page.stop_reason,
                                trigger_job_id=lease.job_id,
                            )
                    can_retry = False
                    if page.stop_reason in {
                        SourceStopReason.RATE_LIMITED,
                        SourceStopReason.UPSTREAM_ERROR,
                    }:
                        session.rollback()
                        try:
                            with session.begin():
                                requests_sent = execution.current_request_counts_in_transaction(
                                    lease,
                                    owner_id=configuration.owner_id,
                                    operation_id=configuration.operation_id,
                                ).collection_cycle
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

    def _configured_adapter_factory(
        self,
        *,
        owner_id: UUID,
        source_key: str,
        connection_id: UUID,
        connection_version: int,
    ) -> SearchAdapterFactory:
        if not (
            source_key in {"hackernews", "google_news", "news_search", "bilibili"}
            or source_key.startswith("rss_")
        ):
            raise UnsupportedSearchSourceError(source_key)
        with self._sessions() as session, session.begin():
            config = require_source_connection_version(
                session,
                owner_id=owner_id,
                source_key=source_key,
                connection_id=connection_id,
                connection_version=connection_version,
            )
        factory = build_search_adapter_factory(source_key, config, owner_id=owner_id)
        if source_key != "bilibili":
            return factory

        def build_with_live_safety(
            before_request: Callable[[int], bool],
            cancelled: Callable[[], bool],
            max_requests: int,
            max_seconds: float,
        ) -> SourceAdapter:
            def safety_cancelled() -> bool:
                if cancelled():
                    return True
                try:
                    with self._sessions() as session, session.begin():
                        require_source_connection_version(
                            session,
                            owner_id=owner_id,
                            source_key=source_key,
                            connection_id=connection_id,
                            connection_version=connection_version,
                        )
                except ApplicationError:
                    return True
                return False

            return factory(before_request, safety_cancelled, max_requests, max_seconds)

        return build_with_live_safety

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
        if isinstance(error, MediaCrawlerPreflightError):
            return self._failure(
                error.code,
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "核对固定 MediaCrawler 版本及工作树后重新提交",
                manual_retry_allowed=False,
            )
        if isinstance(error, MediaCrawlerExecutionError):
            return self._failure(
                error.code,
                (
                    JobFailureCategory.INVALID_RESPONSE
                    if error.code == "mediacrawler_output_invalid"
                    else JobFailureCategory.TRANSIENT
                ),
                "核查本机 MediaCrawler 运行状态和受限输出后手动重试",
            )
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
