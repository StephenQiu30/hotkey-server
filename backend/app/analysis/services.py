from __future__ import annotations

import json
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from typing import Any
from uuid import UUID, uuid4, uuid5

from pydantic import ValidationError
from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from ai.schemas import AiCallError, AiFailureCode
from ai.services import AiService, create_ai_client
from analysis.models import ContentAnnotation
from analysis.prompts import (
    ANALYSIS_OUTPUT_SCHEMA,
    ANALYSIS_PROMPT_VERSION,
    build_analysis_prompt,
    serialize_analysis_data,
)
from analysis.schemas import (
    AnalysisJobScope,
    AnalysisPromptItem,
    AnnotationOutputEnvelope,
    AnnotationOutputItem,
    AnnotationStatus,
    AnnotationWrite,
    WindowAnnotationCountView,
)
from content.schemas import AnalysisPostContentView
from content.services import (
    load_post_comments_for_analysis,
    load_post_versions_for_analysis,
    load_recent_post_versions_for_analysis,
)
from core.config import Settings
from jobs.coverage import CollectionDueWindowService
from jobs.execution import JobCompletion, JobExecutionFailure
from jobs.schemas import (
    JobAcceptanceInput,
    JobFailureCategory,
    JobMessage,
    JobObservationContext,
    JobStatus,
    JobView,
)
from jobs.services import JobService, load_job_execution_configuration
from monitors.services import MonitorTopicService, NormalizedMonitorRules, evaluate_monitor_rules

_ANALYSIS_OPERATION_NAMESPACE = UUID("16cfeef5-e41d-43a2-8212-4e21cc6f4c85")
_MAX_BATCH_ITEMS = 30
_MAX_SERIALIZED_CHARACTERS = 24_000
_MAX_BODY_CHARACTERS = 1_500
_MAX_COMMENTS_PER_POST = 50


@dataclass(frozen=True, slots=True)
class AnalysisExecutionResult:
    completion: JobCompletion
    processed_items: int


def analysis_operation_id(
    *,
    topic_id: UUID,
    topic_rule_version: int,
    content_version_ids: Sequence[UUID],
    prompt_version: str,
) -> UUID:
    if topic_rule_version < 1 or not prompt_version or not content_version_ids:
        raise ValueError("analysis operation identity requires a rule, prompt and content")
    ordered_ids = sorted({str(content_version_id) for content_version_id in content_version_ids})
    if len(ordered_ids) != len(content_version_ids):
        raise ValueError("analysis operation content versions must be distinct")
    name = "\0".join((str(topic_id), str(topic_rule_version), prompt_version, *ordered_ids))
    return uuid5(_ANALYSIS_OPERATION_NAMESPACE, name)


def pack_prompt_batches(
    items: Sequence[AnalysisPromptItem],
) -> tuple[tuple[AnalysisPromptItem, ...], ...]:
    """Pack immutable post text first, then share remaining space across comments."""
    normalized = tuple(
        item.model_copy(
            update={
                "body": item.body[:_MAX_BODY_CHARACTERS] if item.body is not None else None,
                "comments": tuple(item.comments[:_MAX_COMMENTS_PER_POST]),
            }
        )
        for item in items
    )
    base_batches: list[list[AnalysisPromptItem]] = []
    current: list[AnalysisPromptItem] = []
    for item in normalized:
        without_comments = item.model_copy(update={"comments": ()})
        candidate = [*current, without_comments]
        if current and (
            len(candidate) > _MAX_BATCH_ITEMS
            or len(serialize_analysis_data(candidate)) > _MAX_SERIALIZED_CHARACTERS
        ):
            base_batches.append(current)
            current = [without_comments]
        else:
            current = candidate
        if len(serialize_analysis_data(current)) > _MAX_SERIALIZED_CHARACTERS:
            raise ValueError("one analysis item exceeds the serialized character limit")
    if current:
        base_batches.append(current)

    source_items = {item.content_version_id: item for item in normalized}
    return tuple(_add_comments_within_limit(batch, source_items) for batch in base_batches)


def _add_comments_within_limit(
    base_batch: list[AnalysisPromptItem],
    source_items: Mapping[UUID, AnalysisPromptItem],
) -> tuple[AnalysisPromptItem, ...]:
    batch = list(base_batch)
    positions = [0] * len(batch)
    exhausted = [False] * len(batch)
    while not all(exhausted):
        changed = False
        for index, item in enumerate(batch):
            source = source_items[item.content_version_id]
            position = positions[index]
            if exhausted[index] or position >= len(source.comments):
                exhausted[index] = True
                continue
            comment = source.comments[position]
            candidate_item = item.model_copy(update={"comments": (*item.comments, comment)})
            candidate_batch = [*batch[:index], candidate_item, *batch[index + 1 :]]
            if len(serialize_analysis_data(candidate_batch)) <= _MAX_SERIALIZED_CHARACTERS:
                batch = candidate_batch
                positions[index] += 1
                changed = True
                continue
            prefix = _largest_comment_prefix(batch, index=index, comment=comment)
            if prefix:
                batch[index] = item.model_copy(update={"comments": (*item.comments, prefix)})
                changed = True
            exhausted[index] = True
        if not changed:
            break
    return tuple(batch)


def _largest_comment_prefix(
    batch: Sequence[AnalysisPromptItem],
    *,
    index: int,
    comment: str,
) -> str:
    low, high = 0, len(comment)
    while low < high:
        midpoint = (low + high + 1) // 2
        candidate_item = batch[index].model_copy(
            update={"comments": (*batch[index].comments, comment[:midpoint])}
        )
        candidate = [*batch[:index], candidate_item, *batch[index + 1 :]]
        if len(serialize_analysis_data(candidate)) <= _MAX_SERIALIZED_CHARACTERS:
            low = midpoint
        else:
            high = midpoint - 1
    return comment[:low]


def resolve_annotation_results(
    *,
    expected_content_version_ids: Sequence[UUID],
    raw_items: Sequence[Any],
    ai_call_id: UUID,
) -> tuple[AnnotationWrite, ...]:
    grouped: dict[UUID, list[Any]] = {}
    expected = set(expected_content_version_ids)
    for raw_item in raw_items:
        if not isinstance(raw_item, Mapping):
            continue
        raw_id = raw_item.get("content_version_id")
        try:
            content_version_id = UUID(str(raw_id))
        except (TypeError, ValueError):
            continue
        if content_version_id in expected:
            grouped.setdefault(content_version_id, []).append(dict(raw_item))

    resolved: list[AnnotationWrite] = []
    for content_version_id in expected_content_version_ids:
        candidates = grouped.get(content_version_id, [])
        if len(candidates) == 1:
            try:
                output = AnnotationOutputItem.model_validate(candidates[0])
            except ValidationError:
                output = None
            if output is not None and output.content_version_id == content_version_id:
                resolved.append(
                    AnnotationWrite(
                        content_version_id=content_version_id,
                        relevant=output.relevant,
                        relevance_reason=output.relevance_reason,
                        sentiment=output.sentiment,
                        summary=output.summary,
                        viewpoints=output.viewpoints,
                        ai_call_id=ai_call_id,
                        status=AnnotationStatus.ANNOTATED,
                    )
                )
                continue
        resolved.append(
            AnnotationWrite(
                content_version_id=content_version_id,
                ai_call_id=ai_call_id,
                status=AnnotationStatus.UNANALYZED,
            )
        )
    return tuple(resolved)


def analysis_failure(error: AiCallError, *, now: datetime) -> JobExecutionFailure:
    if now.tzinfo is None:
        raise ValueError("analysis failure time must be timezone-aware")
    if error.code is AiFailureCode.RATE_LIMITED:
        return JobExecutionFailure(
            error_code="analysis_rate_limited",
            category=JobFailureCategory.RATE_LIMITED,
            occurred_at=now,
            next_action="等待模型限流恢复后自动重试",
            retry_at=now + timedelta(seconds=60),
            max_attempts=3,
        )
    category = (
        JobFailureCategory.INVALID_RESPONSE
        if error.code is AiFailureCode.INVALID_OUTPUT
        else JobFailureCategory.TRANSIENT
    )
    return JobExecutionFailure(
        error_code=f"analysis_{error.code.value}",
        category=category,
        occurred_at=now,
        next_action="检查本机 Codex app-server 状态与结构化输出后手动重试",
        manual_retry_allowed=True,
    )


class AnalysisService:
    def __init__(self, session: Session) -> None:
        self._session = session

    def window_annotation_counts_in_transaction(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        topic_rule_version: int,
        prompt_version: str,
        content_version_ids: tuple[UUID, ...],
    ) -> WindowAnnotationCountView:
        """Separate missing annotations, failed jobs and malformed model output."""
        if not self._session.in_transaction():
            raise RuntimeError("annotation count reads require the caller's transaction")
        if len(set(content_version_ids)) != len(content_version_ids):
            raise ValueError("content version identities must be distinct")
        if not content_version_ids:
            return WindowAnnotationCountView(
                total_count=0,
                annotated_count=0,
                pending_count=0,
                failed_count=0,
                abnormal_count=0,
            )
        annotations = self._session.scalars(
            select(ContentAnnotation).where(
                ContentAnnotation.owner_id == owner_id,
                ContentAnnotation.topic_id == topic_id,
                ContentAnnotation.topic_rule_version == topic_rule_version,
                ContentAnnotation.prompt_version == prompt_version,
                ContentAnnotation.content_version_id.in_(content_version_ids),
            )
        ).all()
        annotated = {
            row.content_version_id
            for row in annotations
            if row.status == AnnotationStatus.ANNOTATED.value
        }
        abnormal = {
            row.content_version_id
            for row in annotations
            if row.status == AnnotationStatus.UNANALYZED.value
        }
        failed: set[UUID] = set()
        for job in CollectionDueWindowService(self._session).list_analysis_jobs_in_transaction(
            owner_id=owner_id
        ):
            if job.status not in {
                JobStatus.FAILED,
                JobStatus.PARTIALLY_SUCCEEDED,
                JobStatus.CANCELLED,
            }:
                continue
            scope = AnalysisJobScope.from_job_scope(job.scope)
            if (
                scope.topic_id == topic_id
                and scope.topic_rule_version == topic_rule_version
                and scope.prompt_version == prompt_version
            ):
                failed.update(set(scope.content_version_ids) & set(content_version_ids))
        failed.difference_update(annotated | abnormal)
        pending = set(content_version_ids) - annotated - abnormal - failed
        return WindowAnnotationCountView(
            total_count=len(content_version_ids),
            annotated_count=len(annotated),
            pending_count=len(pending),
            failed_count=len(failed),
            abnormal_count=len(abnormal),
        )

    def enqueue_due_batches_in_transaction(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        now: datetime,
    ) -> tuple[JobView, ...]:
        """Accept due topic batches inside the scheduler-owned transaction."""
        if not self._session.in_transaction():
            raise RuntimeError("analysis scanning requires the caller's transaction")
        if now.tzinfo is None:
            raise ValueError("analysis scan time must be timezone-aware")
        rule_version, rules = MonitorTopicService(
            self._session
        ).get_current_topic_rules_in_transaction(owner_id=owner_id, topic_id=topic_id)
        candidates = load_recent_post_versions_for_analysis(
            self._session,
            owner_id=owner_id,
            since=now - timedelta(hours=72),
        )
        matched = tuple(
            item for item in candidates if evaluate_monitor_rules(rules, _post_text(item)).matched
        )
        due = self._missing_posts(
            owner_id=owner_id,
            topic_id=topic_id,
            topic_rule_version=rule_version,
            posts=matched,
        )
        comments = load_post_comments_for_analysis(
            self._session,
            owner_id=owner_id,
            post_content_ids={item.content_id for item in due},
        )
        prompt_items = tuple(
            _prompt_item(
                item,
                comments=tuple(comment.text for comment in comments.get(item.content_id, ())),
            )
            for item in due
        )
        accepted: list[JobView] = []
        for batch in pack_prompt_batches(prompt_items):
            content_version_ids = tuple(
                sorted((item.content_version_id for item in batch), key=str)
            )
            operation_id = analysis_operation_id(
                topic_id=topic_id,
                topic_rule_version=rule_version,
                content_version_ids=content_version_ids,
                prompt_version=ANALYSIS_PROMPT_VERSION,
            )
            accepted.append(
                JobService(self._session, clock=lambda: now).accept_in_transaction(
                    owner_id=owner_id,
                    command=JobAcceptanceInput(
                        operation_id=operation_id,
                        kind="analysis.annotate",
                        observation=JobObservationContext(
                            configuration_ref=f"topic:{topic_id}",
                            configuration_version=rule_version,
                        ),
                        scope={
                            "topic_id": str(topic_id),
                            "topic_rule_version": rule_version,
                            "prompt_version": ANALYSIS_PROMPT_VERSION,
                            "content_version_ids": json.dumps(
                                [str(item) for item in content_version_ids],
                                separators=(",", ":"),
                            ),
                        },
                    ),
                )
            )
        return tuple(accepted)

    def missing_content_version_ids_in_transaction(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        topic_rule_version: int,
        prompt_version: str,
        content_version_ids: Sequence[UUID],
    ) -> tuple[UUID, ...]:
        if not self._session.in_transaction():
            raise RuntimeError("analysis reads require the caller's transaction")
        existing = set(
            self._session.scalars(
                select(ContentAnnotation.content_version_id).where(
                    ContentAnnotation.owner_id == owner_id,
                    ContentAnnotation.topic_id == topic_id,
                    ContentAnnotation.topic_rule_version == topic_rule_version,
                    ContentAnnotation.prompt_version == prompt_version,
                    ContentAnnotation.content_version_id.in_(content_version_ids),
                )
            )
        )
        return tuple(item for item in content_version_ids if item not in existing)

    def persist_results_in_transaction(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        topic_rule_version: int,
        prompt_version: str,
        posts: Mapping[UUID, AnalysisPostContentView],
        results: Sequence[AnnotationWrite],
        created_at: datetime,
    ) -> None:
        if not self._session.in_transaction():
            raise RuntimeError("analysis writes require the caller's transaction")
        if created_at.tzinfo is None:
            raise ValueError("analysis creation time must be timezone-aware")
        for result in results:
            post = posts.get(result.content_version_id)
            if post is None:
                raise ValueError("analysis result references an unfrozen content version")
            self._session.execute(
                insert(ContentAnnotation)
                .values(
                    id=uuid4(),
                    owner_id=owner_id,
                    content_id=post.content_id,
                    content_version_id=result.content_version_id,
                    topic_id=topic_id,
                    topic_rule_version=topic_rule_version,
                    prompt_version=prompt_version,
                    relevant=result.relevant,
                    relevance_reason=result.relevance_reason,
                    sentiment=result.sentiment.value if result.sentiment is not None else None,
                    summary=result.summary,
                    viewpoints=list(result.viewpoints),
                    ai_call_id=result.ai_call_id,
                    status=result.status.value,
                    created_at=created_at,
                )
                .on_conflict_do_nothing(
                    constraint="content_annotations_owner_version_topic_rule_prompt_key"
                )
            )

    def _missing_posts(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        topic_rule_version: int,
        posts: Sequence[AnalysisPostContentView],
    ) -> tuple[AnalysisPostContentView, ...]:
        missing_ids = set(
            self.missing_content_version_ids_in_transaction(
                owner_id=owner_id,
                topic_id=topic_id,
                topic_rule_version=topic_rule_version,
                prompt_version=ANALYSIS_PROMPT_VERSION,
                content_version_ids=tuple(item.content_version_id for item in posts),
            )
        )
        return tuple(item for item in posts if item.content_version_id in missing_ids)


class AnalysisAnnotateExecutor:
    def __init__(
        self,
        sessions: sessionmaker[Session],
        settings: Settings,
        *,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._sessions = sessions
        self._settings = settings
        self._clock = clock or (lambda: datetime.now(UTC))

    def execute(self, message: JobMessage) -> AnalysisExecutionResult:
        scope, rules, posts, items = self._load_execution(message)
        if not items:
            return AnalysisExecutionResult(
                completion=JobCompletion(status=JobStatus.SUCCEEDED),
                processed_items=len(scope.content_version_ids),
            )
        batches = pack_prompt_batches(items)
        if len(batches) != 1:
            raise self._configuration_failure("analysis_batch_scope_invalid")
        client = create_ai_client(self._settings)
        try:
            try:
                with self._sessions() as session:
                    completion = AiService(session, client, clock=self._clock).complete(
                        owner_id=message.owner_id,
                        job_id=message.job_id,
                        purpose="analysis.annotate",
                        prompt_version=scope.prompt_version,
                        prompt=build_analysis_prompt(
                            items=batches[0],
                            match_any=rules.match_any,
                            match_all=rules.match_all,
                            exclude=rules.exclude,
                        ),
                        output_schema=ANALYSIS_OUTPUT_SCHEMA,
                    )
            except AiCallError as error:
                raise analysis_failure(error, now=self._clock()) from error
            if completion.call_id is None:
                raise self._configuration_failure("analysis_ai_call_missing")
            try:
                envelope = AnnotationOutputEnvelope.model_validate(completion.output)
            except ValidationError as error:
                raise JobExecutionFailure(
                    error_code="analysis_invalid_output",
                    category=JobFailureCategory.INVALID_RESPONSE,
                    occurred_at=self._clock(),
                    next_action="检查模型批量输出对象后手动重试",
                    manual_retry_allowed=True,
                ) from error
            results = resolve_annotation_results(
                expected_content_version_ids=tuple(item.content_version_id for item in items),
                raw_items=envelope.items,
                ai_call_id=completion.call_id,
            )
            with self._sessions() as session, session.begin():
                AnalysisService(session).persist_results_in_transaction(
                    owner_id=message.owner_id,
                    topic_id=scope.topic_id,
                    topic_rule_version=scope.topic_rule_version,
                    prompt_version=scope.prompt_version,
                    posts=posts,
                    results=results,
                    created_at=self._clock(),
                )
            invalid_count = sum(result.status is AnnotationStatus.UNANALYZED for result in results)
            job_completion = (
                JobCompletion(status=JobStatus.SUCCEEDED)
                if invalid_count == 0
                else JobCompletion(
                    status=JobStatus.PARTIALLY_SUCCEEDED,
                    failure=JobExecutionFailure(
                        error_code="analysis_items_invalid",
                        category=JobFailureCategory.INVALID_RESPONSE,
                        occurred_at=self._clock(),
                        next_action="随下一提示词版本重新分析未分析条目",
                        manual_retry_allowed=False,
                    ),
                )
            )
            return AnalysisExecutionResult(
                completion=job_completion,
                processed_items=len(scope.content_version_ids),
            )
        finally:
            client.close()

    def _load_execution(
        self,
        message: JobMessage,
    ) -> tuple[
        AnalysisJobScope,
        NormalizedMonitorRules,
        dict[UUID, AnalysisPostContentView],
        tuple[AnalysisPromptItem, ...],
    ]:
        with self._sessions() as session, session.begin():
            configuration = load_job_execution_configuration(session, job_id=message.job_id)
            if configuration is None:
                raise self._configuration_failure("analysis_job_missing")
            try:
                scope = AnalysisJobScope.from_job_scope(configuration.scope)
            except (TypeError, ValueError, ValidationError) as error:
                raise self._configuration_failure("analysis_scope_invalid") from error
            expected_operation_id = analysis_operation_id(
                topic_id=scope.topic_id,
                topic_rule_version=scope.topic_rule_version,
                content_version_ids=scope.content_version_ids,
                prompt_version=scope.prompt_version,
            )
            if (
                configuration.owner_id != message.owner_id
                or configuration.operation_id != message.operation_id
                or configuration.operation_id != expected_operation_id
                or configuration.kind != "analysis.annotate"
                or configuration.observation.configuration_ref != f"topic:{scope.topic_id}"
                or configuration.observation.configuration_version != scope.topic_rule_version
                or configuration.observation.source_key is not None
                or scope.prompt_version != ANALYSIS_PROMPT_VERSION
                or tuple(sorted(scope.content_version_ids, key=str)) != scope.content_version_ids
            ):
                raise self._configuration_failure("analysis_scope_mismatch")
            rules = MonitorTopicService(session).get_topic_rules_in_transaction(
                owner_id=message.owner_id,
                topic_id=scope.topic_id,
                version=scope.topic_rule_version,
            )
            missing_ids = AnalysisService(session).missing_content_version_ids_in_transaction(
                owner_id=message.owner_id,
                topic_id=scope.topic_id,
                topic_rule_version=scope.topic_rule_version,
                prompt_version=scope.prompt_version,
                content_version_ids=scope.content_version_ids,
            )
            loaded_posts = load_post_versions_for_analysis(
                session,
                owner_id=message.owner_id,
                content_version_ids=set(missing_ids),
            )
            posts = {item.content_version_id: item for item in loaded_posts}
            if len(posts) != len(missing_ids):
                raise self._configuration_failure("analysis_content_missing")
            comments = load_post_comments_for_analysis(
                session,
                owner_id=message.owner_id,
                post_content_ids={item.content_id for item in loaded_posts},
            )
            items = tuple(
                _prompt_item(
                    posts[content_version_id],
                    comments=tuple(
                        comment.text
                        for comment in comments.get(posts[content_version_id].content_id, ())
                    ),
                )
                for content_version_id in missing_ids
            )
        return scope, rules, posts, items

    def _configuration_failure(self, code: str) -> JobExecutionFailure:
        return JobExecutionFailure(
            error_code=code,
            category=JobFailureCategory.CONFIGURATION_UNAVAILABLE,
            occurred_at=self._clock(),
            next_action="重新扫描并受理与当前主题和内容版本一致的分析任务",
            manual_retry_allowed=True,
        )


def _post_text(post: AnalysisPostContentView) -> str:
    return "\n".join(part for part in (post.title, post.body) if part)


def _prompt_item(
    post: AnalysisPostContentView,
    *,
    comments: tuple[str, ...],
) -> AnalysisPromptItem:
    return AnalysisPromptItem(
        content_id=post.content_id,
        content_version_id=post.content_version_id,
        title=post.title,
        body=post.body[:_MAX_BODY_CHARACTERS] if post.body is not None else None,
        comments=comments[:_MAX_COMMENTS_PER_POST],
    )
