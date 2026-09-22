from __future__ import annotations

from collections.abc import Callable
from contextlib import AbstractContextManager
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from urllib.parse import urlsplit
from uuid import UUID

from sqlalchemy.orm import Session, sessionmaker

from connections.schemas import (
    ConnectionEvidenceOutcome,
    PersistedReadEvidenceInput,
    SourceEntryPoint,
)
from connections.services import (
    SourceCapabilityEvidenceService,
    require_web_connection_execution,
    resolve_web_connection_execution,
)
from content.schemas import (
    ContentRecordDetailView,
    ContentTruncationReason,
    PersistContentDocumentInput,
)
from content.services import ContentService
from core.errors import ApplicationError
from evidence.schemas import DataClass
from evidence.services import (
    LifecycleService,
    ResourceUnavailableError,
    RetentionPolicyUnavailableError,
    SourceAccessPolicyService,
    SourceAccessUnavailableError,
)
from jobs.execution import (
    ExecutionLease,
    JobExecutionFailure,
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
    CollectionJobKind,
    JobAcceptanceInput,
    JobFailureCategory,
    JobMessage,
    JobObservationContext,
    JobStage,
    JobView,
    UsageAttemptInput,
    UsageKind,
    UsageOutcome,
)
from jobs.services import (
    BudgetPolicyUnavailableError,
    ComponentPolicyUnavailableError,
    JobExecutionConfiguration,
    JobService,
    ResourceBudgetService,
    UsageConflictError,
    load_job_execution_configuration,
    load_job_execution_configuration_by_operation,
)
from sources.adapters.web_targets import normalize_web_url
from sources.contracts import (
    DocumentAdapter,
    SourceCapability,
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


class WebPageCollectionService:
    """Accept a typed webpage job using server-owned execution context."""

    def __init__(self, session: Session, *, clock: Clock | None = None) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def accept_job(
        self,
        *,
        owner_id: UUID,
        operation_id: UUID,
        target_url: str,
    ) -> JobView:
        self._session.rollback()
        with self._session.begin():
            existing = load_job_execution_configuration_by_operation(
                self._session,
                owner_id=owner_id,
                kind=CollectionJobKind.WEBPAGE_COLLECT.value,
                operation_id=operation_id,
            )
            if existing is not None:
                stored_target = existing.scope.get("target_url")
                stored_host = (
                    urlsplit(stored_target).hostname if isinstance(stored_target, str) else None
                )
                if stored_host is None:
                    raise RuntimeError("stored webpage job target is invalid")
                try:
                    normalized_target = normalize_web_url(
                        target_url,
                        allowed_hosts=frozenset({stored_host}),
                    )
                except ValueError as error:
                    raise ApplicationError("idempotency_conflict") from error
                if normalized_target != stored_target:
                    raise ApplicationError("idempotency_conflict")
                return JobService(
                    self._session,
                    clock=self._clock,
                ).accept_in_transaction(
                    owner_id=owner_id,
                    command=JobAcceptanceInput(
                        operation_id=operation_id,
                        kind=existing.kind,
                        observation=existing.observation,
                        scheduled_for_at=None,
                        scope=existing.scope,
                    ),
                )
            connection = resolve_web_connection_execution(
                self._session,
                owner_id=owner_id,
                target_url=target_url,
            )
            return JobService(
                self._session,
                clock=self._clock,
            ).accept_in_transaction(
                owner_id=owner_id,
                command=JobAcceptanceInput(
                    operation_id=operation_id,
                    kind=CollectionJobKind.WEBPAGE_COLLECT,
                    observation=JobObservationContext(
                        configuration_ref=f"connection:{connection.connection_id.hex}",
                        configuration_version=connection.connection_version,
                        source_key="web",
                        source_capability=SourceCapability.PAGE_CONTENT,
                    ),
                    scheduled_for_at=None,
                    scope={
                        "connection_id": str(connection.connection_id),
                        "entry_point": "manual",
                        "target_url": connection.normalized_url,
                    },
                ),
            )


class WebPageFetchService:
    """Fence, meter, and execute one public-web collector call."""

    def __init__(
        self,
        session: Session,
        *,
        lease_seconds: int,
        timeout_seconds: int = 20,
        clock: Clock | None = None,
    ) -> None:
        self._session = session
        self._lease_seconds = lease_seconds
        if not 1 <= timeout_seconds <= 20:
            raise ValueError("timeout_seconds must be between 1 and 20")
        self._timeout_seconds = timeout_seconds
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
            budget.recover_abandoned_attempts_in_transaction(
                owner_id=owner_id,
                operation_id=command.operation_id,
                component_key=_FIRECRAWL_COMPONENT_KEY,
                stage=_FIRECRAWL_STAGE,
                finished_at=started_at,
            )
            connection = require_web_connection_execution(
                self._session,
                owner_id=owner_id,
                connection_id=command.connection_id,
                connection_version=command.connection_version,
                target_url=command.target_url,
            )
            SourceAccessPolicyService(
                self._session,
                clock=self._clock,
            ).require_admission_ready_in_transaction(
                owner_id=owner_id,
                source_key="web",
                capability=SourceCapability.PAGE_CONTENT,
                data_class=DataClass.STRUCTURED,
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

        request = WebPageRequest(
            url=connection.normalized_url,
            timeout_seconds=self._timeout_seconds,
        )
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


class WebPageCollectionExecutor:
    """Execute one accepted webpage job without owning message acknowledgement."""

    def __init__(
        self,
        sessions: sessionmaker[Session],
        *,
        lease_seconds: int,
        adapter_factory: DocumentAdapterFactory,
        timeout_seconds: int = 20,
        clock: Clock | None = None,
    ) -> None:
        self._sessions = sessions
        self._lease_seconds = lease_seconds
        self._adapter_factory = adapter_factory
        self._timeout_seconds = timeout_seconds
        self._clock = clock or (lambda: datetime.now(UTC))

    def execute(self, message: JobMessage, lease: ExecutionLease) -> ExecutionLease:
        if lease.checkpoint_sequence >= 1:
            self._verify_checkpoint(message, lease)
            return lease

        configuration = self._configuration(message)
        connection_id, target_url = self._execution_scope(configuration.scope)
        if configuration.observation.configuration_ref != f"connection:{connection_id.hex}":
            raise self._failure(
                "job_execution_scope_invalid",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "重新提交包含当前网页连接的采集任务",
                manual_retry_allowed=True,
            )
        try:
            with self._sessions() as session:
                fetched = WebPageFetchService(
                    session,
                    lease_seconds=self._lease_seconds,
                    timeout_seconds=self._timeout_seconds,
                    clock=self._clock,
                ).fetch_document(
                    owner_id=configuration.owner_id,
                    lease=lease,
                    command=WebPageFetchInput(
                        operation_id=configuration.operation_id,
                        connection_id=connection_id,
                        connection_version=configuration.observation.configuration_version,
                        target_url=target_url,
                    ),
                    adapter_factory=self._adapter_factory,
                )
        except JobLeaseUnavailableError:
            return lease
        except WebPageBudgetDelayedError as error:
            retry_at = error.decision.retry_at
            if retry_at is None:
                retry_at = self._clock() + timedelta(seconds=30)
            raise self._failure(
                "collector_budget_exhausted",
                JobFailureCategory.RATE_LIMITED,
                "等待采集预算窗口恢复后自动重试",
                manual_retry_allowed=True,
                retry_at=retry_at,
                max_attempts=3,
            ) from error
        except (ComponentPolicyUnavailableError, BudgetPolicyUnavailableError) as error:
            raise self._failure(
                "collector_policy_unavailable",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "配置可用的 Firecrawl 组件与采集预算策略后重试",
                manual_retry_allowed=True,
            ) from error
        except (SourceAccessUnavailableError, RetentionPolicyUnavailableError) as error:
            raise self._failure(
                "source_policy_unavailable",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "批准网页采集与结构化资料保留策略后重试",
                manual_retry_allowed=True,
            ) from error
        except ApplicationError as error:
            raise self._application_failure(error) from error

        if fetched.result.document is None:
            self._record_failed_read(
                owner_id=configuration.owner_id,
                connection_id=connection_id,
                connection_version=configuration.observation.configuration_version,
                attempt_id=fetched.attempt_id,
                stop_reason=fetched.result.stop_reason,
            )
            raise self._source_failure(fetched.result.stop_reason)

        document = fetched.result.document
        try:
            with self._sessions() as session:
                admission = SourceAccessPolicyService(
                    session,
                    clock=self._clock,
                ).admit_payload(
                    owner_id=configuration.owner_id,
                    source_key="web",
                    capability=SourceCapability.PAGE_CONTENT,
                    data_class=DataClass.STRUCTURED,
                    collected_at=document.observed_at,
                    payload={
                        "object_type": "webpage",
                        "request_url": document.request_url,
                        "final_url": document.final_url,
                        "published_at": self._timestamp(document.published_at),
                        "text_scope": document.text_scope,
                        "text_origin": "machine_extracted",
                        "text_origin_ref": document.extractor_version,
                        "title": document.title,
                        "body": document.text,
                        "truncation_reason": (
                            ContentTruncationReason.COLLECTOR_LIMIT.value
                            if document.text_scope == "truncated"
                            else None
                        ),
                    },
                )
                _, renewed = WebPageCommitService(
                    session,
                    lease_seconds=self._lease_seconds,
                    clock=self._clock,
                ).commit_document_page(
                    owner_id=configuration.owner_id,
                    lease=fetched.lease,
                    command=PersistContentDocumentInput(
                        job_id=configuration.job_id,
                        source_operation_id=fetched.attempt_id,
                        connection_id=connection_id,
                        connection_version=configuration.observation.configuration_version,
                        entry_point=SourceEntryPoint.MANUAL,
                        component_name="firecrawl",
                        component_version="2.11.162",
                        admission=admission,
                    ),
                    sequence=1,
                )
        except JobLeaseUnavailableError:
            return fetched.lease
        except (
            SourceAccessUnavailableError,
            RetentionPolicyUnavailableError,
            ResourceUnavailableError,
        ) as error:
            raise self._failure(
                "source_policy_unavailable",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "恢复网页采集与结构化资料保留策略后重试",
                manual_retry_allowed=True,
            ) from error
        except ApplicationError as error:
            raise self._application_failure(error) from error
        return renewed

    def _configuration(self, message: JobMessage) -> JobExecutionConfiguration:
        with self._sessions() as session:
            configuration = load_job_execution_configuration(session, job_id=message.job_id)
        if (
            configuration is None
            or configuration.owner_id != message.owner_id
            or configuration.operation_id != message.operation_id
            or configuration.kind != CollectionJobKind.WEBPAGE_COLLECT.value
            or message.kind != configuration.kind
            or message.configuration_ref != configuration.observation.configuration_ref
            or message.configuration_version != configuration.observation.configuration_version
            or message.source_key != "web"
            or configuration.observation.source_key != "web"
            or message.source_capability is not SourceCapability.PAGE_CONTENT
            or configuration.observation.source_capability is not SourceCapability.PAGE_CONTENT
        ):
            raise self._failure(
                "job_execution_context_invalid",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "重新提交使用当前网页连接配置的采集任务",
                manual_retry_allowed=True,
            )
        return configuration

    def _execution_scope(self, scope: dict[str, str | int | bool | None]) -> tuple[UUID, str]:
        raw_connection_id = scope.get("connection_id")
        target_url = scope.get("target_url")
        if (
            not isinstance(raw_connection_id, str)
            or not isinstance(target_url, str)
            or scope.get("entry_point") != SourceEntryPoint.MANUAL.value
            or set(scope) != {"connection_id", "entry_point", "target_url"}
        ):
            raise self._failure(
                "job_execution_scope_invalid",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "重新提交包含服务端执行上下文的网页采集任务",
                manual_retry_allowed=True,
            )
        try:
            connection_id = UUID(raw_connection_id)
        except ValueError as error:
            raise self._failure(
                "job_execution_scope_invalid",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "重新提交包含有效连接的网页采集任务",
                manual_retry_allowed=True,
            ) from error
        return connection_id, target_url

    def _verify_checkpoint(self, message: JobMessage, lease: ExecutionLease) -> None:
        raw_content_id = lease.checkpoint.get("content_id")
        raw_observation_id = lease.checkpoint.get("observation_id")
        if not isinstance(raw_content_id, str) or not isinstance(raw_observation_id, str):
            raise self._failure(
                "job_checkpoint_invalid",
                JobFailureCategory.INVALID_RESPONSE,
                "检查持久化结果后手动重试任务",
                manual_retry_allowed=True,
            )
        try:
            content_id = UUID(raw_content_id)
            observation_id = UUID(raw_observation_id)
        except ValueError as error:
            raise self._failure(
                "job_checkpoint_invalid",
                JobFailureCategory.INVALID_RESPONSE,
                "检查持久化结果后手动重试任务",
                manual_retry_allowed=True,
            ) from error
        with self._sessions() as session:
            try:
                ContentService(session, clock=self._clock).require_persisted_document_result(
                    owner_id=message.owner_id,
                    job_id=message.job_id,
                    content_id=content_id,
                    observation_id=observation_id,
                )
            except ApplicationError as error:
                raise self._failure(
                    "job_checkpoint_result_missing",
                    JobFailureCategory.INVALID_RESPONSE,
                    "检查缺失的持久化结果后手动重试任务",
                    manual_retry_allowed=True,
                ) from error

    def _record_failed_read(
        self,
        *,
        owner_id: UUID,
        connection_id: UUID,
        connection_version: int,
        attempt_id: UUID,
        stop_reason: SourceStopReason | None,
    ) -> None:
        if stop_reason is None:
            raise RuntimeError("failed webpage result is missing a stop reason")
        try:
            with self._sessions() as session:
                SourceCapabilityEvidenceService(
                    session,
                    clock=self._clock,
                ).record_persisted_read(
                    owner_id=owner_id,
                    command=PersistedReadEvidenceInput(
                        operation_id=attempt_id,
                        connection_id=connection_id,
                        connection_version=connection_version,
                        capability=SourceCapability.PAGE_CONTENT,
                        entry_point=SourceEntryPoint.MANUAL,
                        outcome=ConnectionEvidenceOutcome.FAILED,
                        stop_reason=stop_reason,
                        resource_ref=None,
                        component_name="firecrawl",
                        component_version="2.11.162",
                    ),
                )
        except ApplicationError as error:
            raise self._application_failure(error) from error

    def _source_failure(self, stop_reason: SourceStopReason | None) -> JobExecutionFailure:
        now = self._clock()
        if stop_reason is None:
            return JobExecutionFailure(
                error_code="collector_invalid_response",
                category=JobFailureCategory.INVALID_RESPONSE,
                occurred_at=now,
                next_action="检查采集响应后手动重试",
                manual_retry_allowed=True,
            )
        if stop_reason is SourceStopReason.RATE_LIMITED:
            return JobExecutionFailure(
                error_code="source_rate_limited",
                category=JobFailureCategory.RATE_LIMITED,
                occurred_at=now,
                next_action="等待来源限流恢复后自动重试",
                manual_retry_allowed=True,
                retry_at=now + timedelta(seconds=30),
                max_attempts=3,
            )
        if stop_reason is SourceStopReason.UPSTREAM_ERROR:
            return JobExecutionFailure(
                error_code="source_upstream_unavailable",
                category=JobFailureCategory.TRANSIENT,
                occurred_at=now,
                next_action="等待 Firecrawl 恢复后自动重试",
                manual_retry_allowed=True,
                retry_at=now + timedelta(seconds=30),
                max_attempts=3,
            )
        mapping = {
            SourceStopReason.AUTHENTICATION_REQUIRED: (
                "source_authentication_required",
                JobFailureCategory.AUTHENTICATION_REQUIRED,
                "完成来源认证后手动重试",
            ),
            SourceStopReason.ACCESS_DENIED: (
                "source_access_denied",
                JobFailureCategory.PERMISSION_DENIED,
                "确认目标域名和来源权限后手动重试",
            ),
            SourceStopReason.NOT_FOUND: (
                "source_not_found",
                JobFailureCategory.INVALID_INPUT,
                "检查网页地址后重新提交任务",
            ),
            SourceStopReason.SOURCE_EMPTY: (
                "source_content_empty",
                JobFailureCategory.PARSE_ERROR,
                "确认网页包含可提取正文后手动重试",
            ),
            SourceStopReason.UNSUPPORTED: (
                "collector_unavailable",
                JobFailureCategory.CONFIGURATION_UNAVAILABLE,
                "启用 Firecrawl 采集器后手动重试",
            ),
            SourceStopReason.BUDGET_EXHAUSTED: (
                "collector_response_too_large",
                JobFailureCategory.INVALID_RESPONSE,
                "缩小目标页面后手动重试",
            ),
            SourceStopReason.PROTOCOL_ERROR: (
                "collector_protocol_error",
                JobFailureCategory.INVALID_RESPONSE,
                "检查 Firecrawl 响应兼容性后手动重试",
            ),
            SourceStopReason.CANCELLED: (
                "source_cancelled",
                JobFailureCategory.TRANSIENT,
                "等待来源恢复后手动重试",
            ),
            SourceStopReason.END_OF_RESULTS: (
                "source_content_empty",
                JobFailureCategory.PARSE_ERROR,
                "确认网页包含可提取正文后手动重试",
            ),
        }
        error_code, category, next_action = mapping.get(
            stop_reason,
            (
                "collector_invalid_response",
                JobFailureCategory.INVALID_RESPONSE,
                "检查采集响应后手动重试",
            ),
        )
        return JobExecutionFailure(
            error_code=error_code,
            category=category,
            occurred_at=now,
            next_action=next_action,
            manual_retry_allowed=True,
        )

    def _application_failure(self, error: ApplicationError) -> JobExecutionFailure:
        if error.code == "source_target_not_allowed":
            return self._failure(
                "source_target_not_allowed",
                JobFailureCategory.INVALID_INPUT,
                "使用当前网页连接允许的公开域名重新提交任务",
            )
        return self._failure(
            "source_connection_changed",
            JobFailureCategory.CONFIGURATION_UNAVAILABLE,
            "使用当前网页连接版本重新提交任务",
            manual_retry_allowed=True,
        )

    def _failure(
        self,
        error_code: str,
        category: JobFailureCategory,
        next_action: str,
        *,
        manual_retry_allowed: bool = False,
        retry_at: datetime | None = None,
        max_attempts: int | None = None,
    ) -> JobExecutionFailure:
        return JobExecutionFailure(
            error_code=error_code,
            category=category,
            occurred_at=self._clock(),
            next_action=next_action,
            manual_retry_allowed=manual_retry_allowed,
            retry_at=retry_at,
            max_attempts=max_attempts,
        )

    @staticmethod
    def _timestamp(value: datetime | None) -> str | None:
        if value is None:
            return None
        return value.astimezone(UTC).isoformat().replace("+00:00", "Z")
