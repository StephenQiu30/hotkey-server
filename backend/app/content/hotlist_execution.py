from __future__ import annotations

from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from uuid import UUID

from sqlalchemy.orm import Session, sessionmaker

from connections.services import require_source_connection_version
from content.discovery import KeywordRequestMeter
from content.hotlist import HotlistService
from core.errors import ApplicationError
from evidence.services import RetentionPolicyUnavailableError, SourceAccessUnavailableError
from jobs.execution import (
    ExecutionLease,
    JobCompletion,
    JobExecutionFailure,
    JobExecutionService,
    JobLeaseUnavailableError,
    StaleExecutionLeaseError,
)
from jobs.schemas import JobFailureCategory, JobMessage, JobStatus
from jobs.services import load_job_execution_configuration
from sources.adapters.rsshub_hotlist import RsshubHotlistAdapter
from sources.contracts import SourceCapability, SourcePageState, SourceStopReason

type AdapterFactory = Callable[
    [str, str, frozenset[str], Callable[[int], bool], Callable[[], bool]], RsshubHotlistAdapter
]


class HotlistExecutor:
    def __init__(
        self,
        sessions: sessionmaker[Session],
        *,
        lease_seconds: int,
        clock: Callable[[], datetime] | None = None,
        adapter_factory: AdapterFactory | None = None,
    ) -> None:
        self._sessions = sessions
        self._lease_seconds = lease_seconds
        self._clock = clock or (lambda: datetime.now(UTC))
        self._adapter_factory = adapter_factory or self._make_adapter

    @staticmethod
    def _make_adapter(
        source_key: str,
        feed_url: str,
        allowed_hosts: frozenset[str],
        before_request: Callable[[int], bool],
        cancelled: Callable[[], bool],
    ) -> RsshubHotlistAdapter:
        return RsshubHotlistAdapter(
            source_key=source_key,
            feed_url=feed_url,
            allowed_hosts=allowed_hosts,
            before_request=before_request,
            cancelled=cancelled,
        )

    def _failure(self, code: str, category: JobFailureCategory, action: str) -> JobExecutionFailure:
        return JobExecutionFailure(
            error_code=code,
            category=category,
            occurred_at=self._clock(),
            next_action=action,
            manual_retry_allowed=False,
        )

    def execute(
        self, message: JobMessage, lease: ExecutionLease
    ) -> tuple[ExecutionLease, JobCompletion]:
        with self._sessions() as session:
            configuration = load_job_execution_configuration(session, job_id=message.job_id)
        if (
            configuration is None
            or configuration.owner_id != message.owner_id
            or configuration.operation_id != message.operation_id
            or configuration.kind != "source.hotlist"
            or message.kind != "source.hotlist"
            or configuration.observation.source_capability is not SourceCapability.HOTLIST
            or message.source_capability is not SourceCapability.HOTLIST
            or configuration.observation.source_key != message.source_key
            or configuration.observation.configuration_ref != message.configuration_ref
            or configuration.observation.configuration_version != message.configuration_version
        ):
            raise self._failure(
                "hotlist_context_invalid",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "重新提交当前来源版本的热榜任务",
            )
        if "snapshot_id" in lease.checkpoint:
            return lease, JobCompletion(status=JobStatus.SUCCEEDED)
        source_key = configuration.observation.source_key
        scope = configuration.scope
        try:
            if source_key is None or set(scope) != {
                "connection_id",
                "connection_version",
                "interval_seconds",
            }:
                raise ValueError("invalid hotlist scope")
            connection_id = UUID(str(scope["connection_id"]))
            connection_version = scope["connection_version"]
            if type(connection_version) is not int or connection_version < 1:
                raise ValueError("invalid connection version")
            if configuration.started_at is None:
                raise ValueError("hotlist job has not started")
            with self._sessions() as session, session.begin():
                config = require_source_connection_version(
                    session,
                    owner_id=configuration.owner_id,
                    source_key=source_key,
                    connection_id=connection_id,
                    connection_version=connection_version,
                )
            if config.feed_url is None or tuple(config.allowed_hosts) != ("127.0.0.1",):
                raise ValueError("hotlist connection has no local RSSHub feed")
        except (ValueError, TypeError, ApplicationError) as error:
            raise self._failure(
                "hotlist_configuration_invalid",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "检查来源预设及连接版本后重新提交",
            ) from error

        deadline_at = configuration.started_at + timedelta(seconds=45)
        with self._sessions() as session:
            execution = JobExecutionService(
                session, lease_seconds=self._lease_seconds, clock=self._clock
            )
            if execution.cancellation_requested(lease):
                return lease, JobCompletion(status=JobStatus.SUCCEEDED)
            meter = KeywordRequestMeter(
                session,
                owner_id=configuration.owner_id,
                lease=lease,
                operation_id=configuration.operation_id,
                source_key=source_key,
                connection_id=connection_id,
                connection_version=connection_version,
                component_key=f"collector.{source_key}",
                max_requests=1,
                deadline_at=deadline_at,
                lease_seconds=self._lease_seconds,
                clock=self._clock,
                capability=SourceCapability.HOTLIST,
                stage="hotlist.request",
            )
            try:
                adapter = self._adapter_factory(
                    source_key,
                    config.feed_url,
                    frozenset(config.allowed_hosts),
                    meter.before_request,
                    lambda: execution.cancellation_requested(lease),
                )
                page = adapter.fetch_hotlist()
                if page.state in {SourcePageState.STOPPED, SourcePageState.EMPTY}:
                    meter.fail_pending()
                    assert page.stop_reason is not None
                    if page.stop_reason is SourceStopReason.CANCELLED:
                        return lease, JobCompletion(status=JobStatus.SUCCEEDED)
                    category = {
                        SourceStopReason.RATE_LIMITED: JobFailureCategory.RATE_LIMITED,
                        SourceStopReason.ACCESS_DENIED: JobFailureCategory.PERMISSION_DENIED,
                        SourceStopReason.AUTHENTICATION_REQUIRED: (
                            JobFailureCategory.AUTHENTICATION_REQUIRED
                        ),
                        SourceStopReason.PROTOCOL_ERROR: JobFailureCategory.INVALID_RESPONSE,
                        SourceStopReason.NOT_FOUND: JobFailureCategory.INVALID_INPUT,
                        SourceStopReason.BUDGET_EXHAUSTED: (
                            JobFailureCategory.CONFIGURATION_UNAVAILABLE
                        ),
                    }.get(page.stop_reason, JobFailureCategory.TRANSIENT)
                    raise self._failure(
                        f"hotlist_{page.stop_reason.value if page.stop_reason else 'stopped'}",
                        category,
                        "检查热榜来源、准入政策或请求预算后重新提交",
                    )
                renewed = HotlistService(
                    session, lease_seconds=self._lease_seconds, clock=self._clock
                ).commit_snapshot(
                    owner_id=configuration.owner_id,
                    lease=lease,
                    operation_id=configuration.operation_id,
                    source_key=source_key,
                    connection_id=connection_id,
                    connection_version=connection_version,
                    page=page,
                    meter=meter,
                )
                return renewed, JobCompletion(status=JobStatus.SUCCEEDED)
            except JobExecutionFailure:
                raise
            except (JobLeaseUnavailableError, StaleExecutionLeaseError):
                meter.fail_pending()
                raise
            except (
                ApplicationError,
                SourceAccessUnavailableError,
                RetentionPolicyUnavailableError,
            ) as error:
                meter.fail_pending()
                raise self._failure(
                    "hotlist_policy_unavailable",
                    JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                    "恢复当前热榜来源的连接、准入与留存政策",
                ) from error
            except ValueError as error:
                meter.fail_pending()
                raise self._failure(
                    "hotlist_invalid_payload",
                    JobFailureCategory.INVALID_RESPONSE,
                    "检查热榜载荷及内容入库约束",
                ) from error
            except Exception as error:
                meter.fail_pending()
                raise self._failure(
                    "hotlist_processing_failed",
                    JobFailureCategory.TRANSIENT,
                    "检查热榜任务执行日志后重新提交",
                ) from error
