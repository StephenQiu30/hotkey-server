from __future__ import annotations

from collections import Counter, defaultdict
from collections.abc import Callable, Mapping
from dataclasses import dataclass
from datetime import UTC, datetime, time, timedelta
from typing import Any, cast
from uuid import UUID, uuid4, uuid5
from zoneinfo import ZoneInfo

from sqlalchemy import bindparam, select, text
from sqlalchemy.engine import RowMapping
from sqlalchemy.orm import Session, sessionmaker

from ai.schemas import AiCallError, AiCompletion
from ai.services import AiService, create_ai_client
from core.config import get_settings
from jobs.execution import JobCompletion
from jobs.schemas import (
    JobAcceptanceInput,
    JobMessage,
    JobObservationContext,
    JobStatus,
    JobView,
)
from jobs.services import JobService, load_job_execution_configuration
from reports.models import Report
from reports.prompts import (
    REPORT_OUTPUT_SCHEMA,
    REPORT_PROMPT_VERSION,
    build_report_prompt,
    validated_report_narratives,
)
from reports.render import render_daily_report
from reports.schemas import (
    AnnotationState,
    DailyReportData,
    DailyReportJobScope,
    PreparedDailyReport,
    ReportBuildDataset,
    ReportCommentInput,
    ReportComparison,
    ReportContentItem,
    ReportCoverage,
    ReportGenerator,
    ReportInputManifest,
    ReportKind,
    ReportMetricInput,
    ReportOverview,
    ReportPeriod,
    ReportPostInput,
    ReportRepresentativeComment,
    ReportRiskItem,
    ReportSentiment,
    ReportSourceCoverage,
    ReportStatus,
    ReportView,
    ReportVoiceItem,
    SourceCoverageStatus,
)

REPORT_DAILY_OPERATION_NAMESPACE = UUID("3bbf64aa-03b6-4ff8-99af-f09a19af85a9")
REPORT_TIMEZONE = ZoneInfo("Asia/Shanghai")
REPORT_WATERMARK_GRACE = timedelta(minutes=10)

type DatabaseRow = RowMapping | Mapping[str, Any]


@dataclass(frozen=True, slots=True)
class DailyReportExecutionResult:
    completion: JobCompletion
    report_id: UUID
    report_version: int


def daily_report_operation_id(*, topic_id: UUID, window_start: datetime) -> UUID:
    if window_start.utcoffset() != timedelta(0):
        raise ValueError("daily report window start must be UTC")
    identity = f"{topic_id}:{window_start.isoformat()}"
    return uuid5(REPORT_DAILY_OPERATION_NAMESPACE, identity)


def previous_daily_window(now: datetime, *, report_time: time) -> tuple[datetime, datetime]:
    if now.tzinfo is None or report_time.tzinfo is not None:
        raise ValueError("daily report scheduling requires aware now and local report time")
    local_now = now.astimezone(REPORT_TIMEZONE)
    window_end_local = datetime.combine(
        local_now.date(),
        time.min,
        tzinfo=REPORT_TIMEZONE,
    )
    window_start_local = window_end_local - timedelta(days=1)
    return window_start_local.astimezone(UTC), window_end_local.astimezone(UTC)


def should_wait_for_report_watermark(
    *,
    now: datetime,
    due_at: datetime,
    unanalyzed_count: int,
) -> bool:
    if now.tzinfo is None or due_at.tzinfo is None or unanalyzed_count < 0:
        raise ValueError("report watermark requires aware times and a non-negative count")
    return unanalyzed_count > 0 and now < due_at + REPORT_WATERMARK_GRACE


def _post_sort_key(post: ReportPostInput) -> tuple[int, float, str]:
    return (-post.metrics.interaction_count, -post.occurred_at.timestamp(), str(post.content_id))


def _comment_sort_key(comment: ReportCommentInput) -> tuple[int, float, str]:
    return (
        -comment.metrics.interaction_count,
        -comment.occurred_at.timestamp(),
        str(comment.content_id),
    )


def _excerpt(value: str, *, maximum: int = 160) -> str:
    normalized = " ".join(value.split())
    if len(normalized) <= maximum:
        return normalized
    return f"{normalized[: maximum - 1]}…"


def _manifest(dataset: ReportBuildDataset) -> ReportInputManifest:
    current_posts = tuple(post for post in dataset.posts if post.period is ReportPeriod.CURRENT)
    return ReportInputManifest(
        content_version_ids=tuple(
            sorted((post.content_version_id for post in dataset.posts), key=str)
        ),
        annotation_ids=tuple(
            sorted(
                (post.annotation_id for post in dataset.posts if post.annotation_id is not None),
                key=str,
            )
        ),
        observation_ids=tuple(sorted((post.observation_id for post in dataset.posts), key=str)),
        comment_content_version_ids=tuple(
            sorted((comment.content_version_id for comment in dataset.comments), key=str)
        ),
        source_coverage=tuple(sorted(dataset.source_coverage, key=lambda item: item.source_key)),
        discovered_at_count=sum(post.published_at is None for post in current_posts),
        unanalyzed_count=sum(
            post.annotation_state is not AnnotationState.ANNOTATED for post in current_posts
        ),
    )


def _report_data(
    *,
    topic_id: UUID,
    topic_name: str,
    window_start: datetime,
    window_end: datetime,
    cutoff_at: datetime,
    dataset: ReportBuildDataset,
    manifest: ReportInputManifest,
) -> DailyReportData:
    relevant_current = tuple(
        sorted(
            (
                post
                for post in dataset.posts
                if post.period is ReportPeriod.CURRENT
                and post.annotation_state is AnnotationState.ANNOTATED
                and post.relevant is True
            ),
            key=_post_sort_key,
        )
    )
    relevant_previous = tuple(
        post
        for post in dataset.posts
        if post.period is ReportPeriod.PREVIOUS
        and post.annotation_state is AnnotationState.ANNOTATED
        and post.relevant is True
    )
    current_ids = {post.content_id for post in relevant_current}
    previous_ids = {post.content_id for post in relevant_previous}
    current_comments = tuple(
        comment
        for comment in dataset.comments
        if comment.period is ReportPeriod.CURRENT and comment.post_content_id in current_ids
    )
    previous_comments = tuple(
        comment
        for comment in dataset.comments
        if comment.period is ReportPeriod.PREVIOUS and comment.post_content_id in previous_ids
    )
    comments_by_post: dict[UUID, list[ReportCommentInput]] = defaultdict(list)
    for comment in current_comments:
        comments_by_post[comment.post_content_id].append(comment)
    for comments in comments_by_post.values():
        comments.sort(key=_comment_sort_key)

    platform_distribution = dict(
        sorted(Counter(post.source_key for post in relevant_current).items())
    )
    sentiment_counts = Counter(post.sentiment for post in relevant_current)
    sentiment_distribution = {
        sentiment: sentiment_counts[sentiment]
        for sentiment in (
            ReportSentiment.POSITIVE,
            ReportSentiment.NEUTRAL,
            ReportSentiment.NEGATIVE,
        )
    }

    top_posts = relevant_current[:10]
    citations = {post.content_id: f"c{index}" for index, post in enumerate(top_posts, start=1)}
    top_contents = tuple(
        ReportContentItem(
            citation=citations[post.content_id],
            content_id=post.content_id,
            content_version_id=post.content_version_id,
            title=post.title or _excerpt(post.body or "无标题", maximum=80),
            summary=cast(str, post.summary),
            sentiment=cast(ReportSentiment, post.sentiment),
            source_key=post.source_key,
            url=post.url,
            interaction_count=post.metrics.interaction_count,
            representative_comments=tuple(
                ReportRepresentativeComment(
                    content_id=comment.content_id,
                    content_version_id=comment.content_version_id,
                    text=_excerpt(comment.text),
                    interaction_count=comment.metrics.interaction_count,
                )
                for comment in comments_by_post.get(post.content_id, ())[:2]
            ),
        )
        for post in top_posts
    )
    risks = tuple(
        ReportRiskItem(
            citation=citations[post.content_id],
            title=post.title or _excerpt(post.body or "无标题", maximum=80),
            url=post.url,
            reason=cast(str, post.relevance_reason),
            interaction_count=post.metrics.interaction_count,
        )
        for post in top_posts
        if post.sentiment is ReportSentiment.NEGATIVE and post.metrics.interaction_count > 0
    )[:5]

    voice_candidates = sorted(
        (
            comment
            for comment in current_comments
            if comment.post_content_id in citations and comment.text.strip()
        ),
        key=_comment_sort_key,
    )
    voices = tuple(
        ReportVoiceItem(
            citation=citations[comment.post_content_id],
            excerpt=_excerpt(comment.text),
            url=next(post.url for post in top_posts if post.content_id == comment.post_content_id),
        )
        for comment in voice_candidates[:5]
    )
    if not voices:
        voices = tuple(
            ReportVoiceItem(
                citation=citations[post.content_id],
                excerpt=_excerpt(post.body or post.title or "无正文"),
                url=post.url,
            )
            for post in top_posts[:5]
        )

    return DailyReportData(
        topic_id=topic_id,
        topic_name=topic_name,
        window_start=window_start,
        window_end=window_end,
        cutoff_at=cutoff_at,
        overview=ReportOverview(
            posts=ReportComparison(
                current=len(relevant_current),
                previous=len(relevant_previous),
                delta=len(relevant_current) - len(relevant_previous),
            ),
            comments=ReportComparison(
                current=len(current_comments),
                previous=len(previous_comments),
                delta=len(current_comments) - len(previous_comments),
            ),
            platform_distribution=platform_distribution,
            sentiment_distribution=sentiment_distribution,
        ),
        top_contents=top_contents,
        risks=risks,
        voices=voices,
        coverage=ReportCoverage(
            sources=manifest.source_coverage,
            discovered_at_count=manifest.discovered_at_count,
            unanalyzed_count=manifest.unanalyzed_count,
        ),
    )


def prepare_daily_report(
    *,
    owner_id: UUID,
    topic_id: UUID,
    topic_name: str,
    window_start: datetime,
    window_end: datetime,
    cutoff_at: datetime,
    dataset: ReportBuildDataset,
    previous: PreparedDailyReport | None = None,
) -> PreparedDailyReport:
    """Freeze a first version or deterministically regenerate from the prior snapshot."""
    if previous is not None:
        if (
            previous.owner_id != owner_id
            or previous.topic_id != topic_id
            or previous.window_start != window_start
            or previous.window_end != window_end
        ):
            raise ValueError("previous report does not belong to the requested window")
        version = previous.version + 1
        cutoff_at = previous.cutoff_at
        manifest = previous.input_manifest
        data = previous.data.model_copy(update={"narratives": {}})
    else:
        version = 1
        manifest = _manifest(dataset)
        data = _report_data(
            topic_id=topic_id,
            topic_name=topic_name,
            window_start=window_start,
            window_end=window_end,
            cutoff_at=cutoff_at,
            dataset=dataset,
            manifest=manifest,
        )
    return PreparedDailyReport(
        owner_id=owner_id,
        topic_id=topic_id,
        version=version,
        window_start=window_start,
        window_end=window_end,
        cutoff_at=cutoff_at,
        input_manifest=manifest,
        data=data,
        body_markdown=render_daily_report(data),
    )


def apply_model_narratives(
    data: DailyReportData,
    complete: Callable[[str, Mapping[str, Any]], AiCompletion],
) -> DailyReportData:
    """Use only linked, checked sentences; keep deterministic data on AI failure."""
    if not any(item.url is not None for item in data.top_contents):
        return data
    try:
        completion = complete(build_report_prompt(data), REPORT_OUTPUT_SCHEMA)
    except AiCallError:
        return data
    narratives = validated_report_narratives(completion.output, data)
    return data.model_copy(update={"narratives": narratives}) if narratives else data


class ReportService:
    def __init__(
        self,
        session: Session,
        *,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def enqueue_due_in_transaction(self, *, now: datetime) -> tuple[JobView, ...]:
        """Accept daily report jobs after the topic time and watermark wait."""
        if not self._session.in_transaction():
            raise RuntimeError("report scanning requires the caller's transaction")
        if now.tzinfo is None:
            raise ValueError("report scan time must be timezone-aware")
        local_now = now.astimezone(REPORT_TIMEZONE)
        topics = self._session.execute(
            text(
                """
                SELECT id, owner_id, current_version, report_time
                FROM monitor_topics
                WHERE status = 'active' AND readiness_status = 'ready'
                ORDER BY owner_id, id
                FOR UPDATE SKIP LOCKED
                """
            )
        ).mappings()
        accepted: list[JobView] = []
        for row in topics:
            topic_id = _uuid(row, "id")
            owner_id = _uuid(row, "owner_id")
            topic_report_time = row["report_time"]
            if not isinstance(topic_report_time, time):
                raise RuntimeError("topic report time has an invalid database value")
            due_local = datetime.combine(
                local_now.date(),
                topic_report_time,
                tzinfo=REPORT_TIMEZONE,
            )
            if local_now < due_local:
                continue
            window_start, window_end = previous_daily_window(
                now,
                report_time=topic_report_time,
            )
            if self._has_final_report(
                owner_id=owner_id,
                topic_id=topic_id,
                window_start=window_start,
            ):
                continue
            pending = self._load_posts(
                owner_id=owner_id,
                topic_id=topic_id,
                window_start=window_start,
                window_end=window_end,
                cutoff_at=now.astimezone(UTC),
            )
            unanalyzed_count = sum(
                post.period is ReportPeriod.CURRENT
                and post.annotation_state is not AnnotationState.ANNOTATED
                for post in pending
            )
            if should_wait_for_report_watermark(
                now=local_now,
                due_at=due_local,
                unanalyzed_count=unanalyzed_count,
            ):
                continue
            accepted.append(
                JobService(self._session, clock=lambda: now).accept_in_transaction(
                    owner_id=owner_id,
                    command=JobAcceptanceInput(
                        operation_id=daily_report_operation_id(
                            topic_id=topic_id,
                            window_start=window_start,
                        ),
                        kind="report.daily",
                        observation=JobObservationContext(
                            configuration_ref=f"topic:{topic_id}",
                            configuration_version=_int(row, "current_version"),
                        ),
                        scheduled_for_at=due_local.astimezone(UTC),
                        scope={
                            "topic_id": str(topic_id),
                            "window_start": window_start.isoformat(),
                            "window_end": window_end.isoformat(),
                        },
                    ),
                )
            )
        return tuple(accepted)

    def generate_daily_in_transaction(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        window_start: datetime,
        window_end: datetime,
        cutoff_at: datetime,
        narrate: Callable[[DailyReportData], DailyReportData] | None = None,
    ) -> ReportView:
        """Generate once for a job; redelivery returns the existing final version."""
        return self._generate_daily_in_transaction(
            owner_id=owner_id,
            topic_id=topic_id,
            window_start=window_start,
            window_end=window_end,
            cutoff_at=cutoff_at,
            regenerate=False,
            narrate=narrate,
        )

    def regenerate_daily_in_transaction(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        window_start: datetime,
        window_end: datetime,
        cutoff_at: datetime,
    ) -> ReportView:
        """Create a new version while reusing the window's first frozen input."""
        return self._generate_daily_in_transaction(
            owner_id=owner_id,
            topic_id=topic_id,
            window_start=window_start,
            window_end=window_end,
            cutoff_at=cutoff_at,
            regenerate=True,
        )

    def _generate_daily_in_transaction(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        window_start: datetime,
        window_end: datetime,
        cutoff_at: datetime,
        regenerate: bool,
        narrate: Callable[[DailyReportData], DailyReportData] | None = None,
    ) -> ReportView:
        if not self._session.in_transaction():
            raise RuntimeError("report generation requires the caller's transaction")
        if any(
            value.utcoffset() != timedelta(0) for value in (window_start, window_end, cutoff_at)
        ):
            raise ValueError("report persistence times must be UTC")
        existing = self._session.scalars(
            select(Report)
            .where(
                Report.owner_id == owner_id,
                Report.topic_id == topic_id,
                Report.kind == ReportKind.DAILY.value,
                Report.window_start == window_start,
            )
            .order_by(Report.version.desc())
            .with_for_update()
        ).first()
        if existing is not None and not regenerate:
            return self._view(existing)
        topic_name = self._topic_name(owner_id=owner_id, topic_id=topic_id)
        previous = self._prepared_from_model(existing) if existing is not None else None
        dataset = (
            ReportBuildDataset(posts=(), comments=(), source_coverage=())
            if previous is not None
            else self._load_dataset(
                owner_id=owner_id,
                topic_id=topic_id,
                window_start=window_start,
                window_end=window_end,
                cutoff_at=cutoff_at,
            )
        )
        prepared = prepare_daily_report(
            owner_id=owner_id,
            topic_id=topic_id,
            topic_name=topic_name,
            window_start=window_start,
            window_end=window_end,
            cutoff_at=cutoff_at,
            dataset=dataset,
            previous=previous,
        )
        data = narrate(prepared.data) if narrate is not None else prepared.data
        created_at = max(self._clock().astimezone(UTC), prepared.cutoff_at)
        model = Report(
            id=uuid4(),
            owner_id=owner_id,
            topic_id=topic_id,
            kind=ReportKind.DAILY.value,
            window_start=window_start,
            window_end=window_end,
            cutoff_at=prepared.cutoff_at,
            version=prepared.version,
            status=ReportStatus.FINAL.value,
            generator=(
                ReportGenerator.MODEL if data.narratives else ReportGenerator.TEMPLATE
            ).value,
            input_manifest=prepared.input_manifest.model_dump(mode="json"),
            data=data.model_dump(mode="json"),
            body_markdown=render_daily_report(data),
            created_at=created_at,
        )
        self._session.add(model)
        self._session.flush()
        return self._view(model)

    def _has_final_report(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        window_start: datetime,
    ) -> bool:
        return (
            self._session.scalar(
                select(Report.id).where(
                    Report.owner_id == owner_id,
                    Report.topic_id == topic_id,
                    Report.kind == ReportKind.DAILY.value,
                    Report.window_start == window_start,
                    Report.status == ReportStatus.FINAL.value,
                )
            )
            is not None
        )

    def _topic_name(self, *, owner_id: UUID, topic_id: UUID) -> str:
        name = self._session.scalar(
            text(
                """
                SELECT name
                FROM monitor_topics
                WHERE owner_id = :owner_id AND id = :topic_id
                """
            ),
            {"owner_id": owner_id, "topic_id": topic_id},
        )
        if not isinstance(name, str):
            raise ValueError("report topic does not exist")
        return name

    def _load_dataset(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        window_start: datetime,
        window_end: datetime,
        cutoff_at: datetime,
    ) -> ReportBuildDataset:
        posts = self._load_posts(
            owner_id=owner_id,
            topic_id=topic_id,
            window_start=window_start,
            window_end=window_end,
            cutoff_at=cutoff_at,
        )
        relevant_post_ids = {
            post.content_id
            for post in posts
            if post.annotation_state is AnnotationState.ANNOTATED and post.relevant is True
        }
        return ReportBuildDataset(
            posts=posts,
            comments=self._load_comments(
                owner_id=owner_id,
                post_ids=relevant_post_ids,
                window_start=window_start,
                window_end=window_end,
                cutoff_at=cutoff_at,
            ),
            source_coverage=self._load_source_coverage(
                owner_id=owner_id,
                topic_id=topic_id,
                window_start=window_start,
                window_end=window_end,
                cutoff_at=cutoff_at,
            ),
        )

    def _load_posts(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        window_start: datetime,
        window_end: datetime,
        cutoff_at: datetime,
    ) -> tuple[ReportPostInput, ...]:
        rows = self._session.execute(
            text(
                """
                WITH topic_discoveries AS (
                    SELECT cd.content_id, min(cd.first_observed_at) AS first_observed_at
                    FROM content_discoveries AS cd
                    JOIN jobs AS discovery_job
                      ON discovery_job.owner_id = cd.owner_id
                     AND discovery_job.id = cd.job_id
                    WHERE cd.owner_id = :owner_id
                      AND discovery_job.configuration_ref = :configuration_ref
                      AND cd.first_observed_at <= :cutoff_at
                    GROUP BY cd.content_id
                ),
                observations AS (
                    SELECT
                        observation.*,
                        row_number() OVER (
                            PARTITION BY observation.content_id
                            ORDER BY observation.observed_at DESC,
                                     observation.received_at DESC,
                                     observation.id DESC
                        ) AS position
                    FROM content_observations AS observation
                    JOIN topic_discoveries AS discovery
                      ON discovery.content_id = observation.content_id
                    WHERE observation.owner_id = :owner_id
                      AND observation.content_version_id IS NOT NULL
                      AND observation.observed_at <= :cutoff_at
                )
                SELECT
                    record.id AS content_id,
                    record.source_key,
                    version.id AS content_version_id,
                    version.title,
                    version.body,
                    observation.id AS observation_id,
                    observation.published_at,
                    discovery.first_observed_at,
                    coalesce(observation.published_at, discovery.first_observed_at) AS occurred_at,
                    coalesce(observation.canonical_url, observation.final_url) AS url,
                    observation.like_count,
                    observation.comment_count,
                    observation.repost_count,
                    observation.view_count,
                    observation.play_count,
                    observation.danmaku_count,
                    annotation.id AS annotation_id,
                    annotation.status AS annotation_status,
                    annotation.relevant,
                    annotation.sentiment,
                    annotation.summary,
                    annotation.relevance_reason,
                    annotation.viewpoints
                FROM topic_discoveries AS discovery
                JOIN content_records AS record
                  ON record.owner_id = :owner_id
                 AND record.id = discovery.content_id
                 AND record.object_type = 'post'
                JOIN observations AS observation
                  ON observation.content_id = record.id
                 AND observation.position = 1
                JOIN content_versions AS version
                  ON version.owner_id = observation.owner_id
                 AND version.id = observation.content_version_id
                LEFT JOIN LATERAL (
                    SELECT item.*
                    FROM content_annotations AS item
                    WHERE item.owner_id = :owner_id
                      AND item.topic_id = :topic_id
                      AND item.content_version_id = version.id
                      AND item.created_at <= :cutoff_at
                    ORDER BY item.created_at DESC, item.id DESC
                    LIMIT 1
                ) AS annotation ON true
                WHERE coalesce(observation.published_at, discovery.first_observed_at)
                      >= :previous_start
                  AND coalesce(observation.published_at, discovery.first_observed_at)
                      < :window_end
                ORDER BY occurred_at, record.id
                """
            ),
            {
                "owner_id": owner_id,
                "topic_id": topic_id,
                "configuration_ref": f"topic:{topic_id}",
                "previous_start": window_start - timedelta(days=1),
                "window_start": window_start,
                "window_end": window_end,
                "cutoff_at": cutoff_at,
            },
        ).mappings()
        return tuple(self._post_from_row(row, window_start=window_start) for row in rows)

    def _post_from_row(
        self,
        row: DatabaseRow,
        *,
        window_start: datetime,
    ) -> ReportPostInput:
        status = row["annotation_status"]
        annotation_state = (
            AnnotationState.MISSING if status is None else AnnotationState(str(status))
        )
        occurred_at = _datetime(row, "occurred_at")
        raw_viewpoints = row["viewpoints"]
        viewpoints = (
            tuple(str(item) for item in raw_viewpoints) if isinstance(raw_viewpoints, list) else ()
        )
        return ReportPostInput(
            period=(ReportPeriod.CURRENT if occurred_at >= window_start else ReportPeriod.PREVIOUS),
            content_id=_uuid(row, "content_id"),
            content_version_id=_uuid(row, "content_version_id"),
            observation_id=_uuid(row, "observation_id"),
            annotation_id=_optional_uuid(row, "annotation_id"),
            source_key=str(row["source_key"]),
            title=_optional_str(row, "title"),
            body=_optional_str(row, "body"),
            url=_optional_str(row, "url"),
            published_at=_optional_datetime(row, "published_at"),
            first_observed_at=_datetime(row, "first_observed_at"),
            occurred_at=occurred_at,
            annotation_state=annotation_state,
            relevant=_optional_bool(row, "relevant"),
            sentiment=(
                ReportSentiment(str(row["sentiment"])) if row["sentiment"] is not None else None
            ),
            summary=_optional_str(row, "summary"),
            relevance_reason=_optional_str(row, "relevance_reason"),
            viewpoints=viewpoints,
            metrics=_metrics(row),
        )

    def _load_comments(
        self,
        *,
        owner_id: UUID,
        post_ids: set[UUID],
        window_start: datetime,
        window_end: datetime,
        cutoff_at: datetime,
    ) -> tuple[ReportCommentInput, ...]:
        if not post_ids:
            return ()
        statement = text(
            """
            WITH first_discovery AS (
                SELECT content_id, min(first_observed_at) AS first_observed_at
                FROM content_discoveries
                WHERE owner_id = :owner_id AND first_observed_at <= :cutoff_at
                GROUP BY content_id
            ),
            observations AS (
                SELECT
                    observation.*,
                    row_number() OVER (
                        PARTITION BY observation.content_id
                        ORDER BY observation.observed_at DESC,
                                 observation.received_at DESC,
                                 observation.id DESC
                    ) AS position
                FROM content_observations AS observation
                WHERE observation.owner_id = :owner_id
                  AND observation.content_version_id IS NOT NULL
                  AND observation.observed_at <= :cutoff_at
            )
            SELECT
                thread.post_content_id,
                record.id AS content_id,
                version.id AS content_version_id,
                observation.id AS observation_id,
                coalesce(version.body, version.title) AS text,
                coalesce(observation.published_at, discovery.first_observed_at) AS occurred_at,
                observation.like_count,
                observation.comment_count,
                observation.repost_count,
                observation.view_count,
                observation.play_count,
                observation.danmaku_count
            FROM content_threads AS thread
            JOIN content_records AS record
              ON record.owner_id = thread.owner_id
             AND record.id = thread.content_id
             AND record.object_type = 'comment'
            JOIN first_discovery AS discovery ON discovery.content_id = record.id
            JOIN observations AS observation
              ON observation.content_id = record.id
             AND observation.position = 1
            JOIN content_versions AS version
              ON version.owner_id = observation.owner_id
             AND version.id = observation.content_version_id
            WHERE thread.owner_id = :owner_id
              AND thread.post_content_id IN :post_ids
              AND coalesce(observation.published_at, discovery.first_observed_at)
                  >= :previous_start
              AND coalesce(observation.published_at, discovery.first_observed_at)
                  < :window_end
              AND coalesce(version.body, version.title) IS NOT NULL
            ORDER BY occurred_at, record.id
            """
        ).bindparams(bindparam("post_ids", expanding=True))
        rows = self._session.execute(
            statement,
            {
                "owner_id": owner_id,
                "post_ids": tuple(sorted(post_ids, key=str)),
                "previous_start": window_start - timedelta(days=1),
                "window_end": window_end,
                "cutoff_at": cutoff_at,
            },
        ).mappings()
        comments: list[ReportCommentInput] = []
        for row in rows:
            occurred_at = _datetime(row, "occurred_at")
            comments.append(
                ReportCommentInput(
                    period=(
                        ReportPeriod.CURRENT
                        if occurred_at >= window_start
                        else ReportPeriod.PREVIOUS
                    ),
                    post_content_id=_uuid(row, "post_content_id"),
                    content_id=_uuid(row, "content_id"),
                    content_version_id=_uuid(row, "content_version_id"),
                    observation_id=_uuid(row, "observation_id"),
                    text=str(row["text"]),
                    occurred_at=occurred_at,
                    metrics=_metrics(row),
                )
            )
        return tuple(comments)

    def _load_source_coverage(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        window_start: datetime,
        window_end: datetime,
        cutoff_at: datetime,
    ) -> tuple[ReportSourceCoverage, ...]:
        rows = self._session.execute(
            text(
                """
                SELECT
                    schedule.source_key,
                    count(job.id) FILTER (WHERE job.status = 'succeeded') AS succeeded_jobs,
                    count(job.id) FILTER (
                        WHERE job.status = 'partially_succeeded'
                    ) AS partial_jobs,
                    count(job.id) FILTER (
                        WHERE job.status IN ('failed', 'cancelled')
                    ) AS failed_jobs,
                    count(job.id) FILTER (
                        WHERE job.status IN ('queued', 'running')
                    ) AS incomplete_jobs
                FROM monitor_schedules AS schedule
                LEFT JOIN jobs AS job
                  ON job.owner_id = schedule.owner_id
                 AND job.configuration_ref = :configuration_ref
                 AND job.source_key = schedule.source_key
                 AND job.kind IN ('keyword.search', 'source.comments')
                 AND job.created_at <= :cutoff_at
                 AND job.scope ? 'starts_at'
                 AND job.scope ? 'ends_at'
                 AND (job.scope ->> 'starts_at')::timestamptz < :window_end
                 AND (job.scope ->> 'ends_at')::timestamptz > :window_start
                WHERE schedule.owner_id = :owner_id
                  AND schedule.topic_id = :topic_id
                  AND schedule.enabled
                GROUP BY schedule.source_key
                ORDER BY schedule.source_key
                """
            ),
            {
                "owner_id": owner_id,
                "topic_id": topic_id,
                "configuration_ref": f"topic:{topic_id}",
                "window_start": window_start,
                "window_end": window_end,
                "cutoff_at": cutoff_at,
            },
        ).mappings()
        coverage: list[ReportSourceCoverage] = []
        for row in rows:
            succeeded = _int(row, "succeeded_jobs")
            partial = _int(row, "partial_jobs")
            failed = _int(row, "failed_jobs")
            incomplete = _int(row, "incomplete_jobs")
            if succeeded + partial + failed + incomplete == 0:
                status = SourceCoverageStatus.MISSING
            elif incomplete:
                status = SourceCoverageStatus.INCOMPLETE
            elif failed and not succeeded and not partial:
                status = SourceCoverageStatus.FAILED
            elif partial or failed:
                status = SourceCoverageStatus.PARTIAL
            else:
                status = SourceCoverageStatus.SUCCEEDED
            coverage.append(
                ReportSourceCoverage(
                    source_key=str(row["source_key"]),
                    status=status,
                    succeeded_jobs=succeeded,
                    partial_jobs=partial,
                    failed_jobs=failed,
                    incomplete_jobs=incomplete,
                )
            )
        return tuple(coverage)

    @staticmethod
    def _prepared_from_model(model: Report) -> PreparedDailyReport:
        return PreparedDailyReport(
            owner_id=model.owner_id,
            topic_id=model.topic_id,
            version=model.version,
            window_start=model.window_start,
            window_end=model.window_end,
            cutoff_at=model.cutoff_at,
            input_manifest=ReportInputManifest.model_validate(model.input_manifest),
            data=DailyReportData.model_validate(model.data),
            body_markdown=model.body_markdown,
        )

    @staticmethod
    def _view(model: Report) -> ReportView:
        return ReportView(
            id=model.id,
            owner_id=model.owner_id,
            topic_id=model.topic_id,
            kind=ReportKind(model.kind),
            window_start=model.window_start,
            window_end=model.window_end,
            cutoff_at=model.cutoff_at,
            version=model.version,
            status=ReportStatus(model.status),
            generator=ReportGenerator(model.generator),
            input_manifest=ReportInputManifest.model_validate(model.input_manifest),
            data=DailyReportData.model_validate(model.data),
            body_markdown=model.body_markdown,
            created_at=model.created_at,
        )


class DailyReportExecutor:
    def __init__(
        self,
        sessions: sessionmaker[Session],
        *,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._sessions = sessions
        self._clock = clock or (lambda: datetime.now(UTC))

    def execute(self, message: JobMessage) -> DailyReportExecutionResult:
        if message.kind != "report.daily":
            raise ValueError("daily report executor received another task kind")
        cutoff_at = self._clock().astimezone(UTC)
        with self._sessions() as session, session.begin():
            configuration = load_job_execution_configuration(session, job_id=message.job_id)
            if (
                configuration is None
                or configuration.owner_id != message.owner_id
                or configuration.operation_id != message.operation_id
                or configuration.kind != message.kind
                or configuration.observation.configuration_ref != message.configuration_ref
                or configuration.observation.configuration_version != message.configuration_version
            ):
                raise ValueError("daily report job configuration does not match the message")
            scope = DailyReportJobScope.from_job_scope(configuration.scope)
            if message.configuration_ref != f"topic:{scope.topic_id}":
                raise ValueError("daily report topic does not match the job configuration")
            try:
                client = create_ai_client(get_settings())
            except Exception:
                client = None
            try:

                def narrate(data: DailyReportData) -> DailyReportData:
                    if client is None:
                        return data
                    ai_service = AiService(session, client, clock=self._clock)
                    return apply_model_narratives(
                        data,
                        lambda prompt, schema: ai_service.complete(
                            owner_id=message.owner_id,
                            job_id=message.job_id,
                            purpose="report.daily",
                            prompt_version=REPORT_PROMPT_VERSION,
                            prompt=prompt,
                            output_schema=schema,
                        ),
                    )

                report = ReportService(
                    session, clock=lambda: cutoff_at
                ).generate_daily_in_transaction(
                    owner_id=message.owner_id,
                    topic_id=scope.topic_id,
                    window_start=scope.window_start,
                    window_end=scope.window_end,
                    cutoff_at=cutoff_at,
                    narrate=narrate,
                )
            finally:
                if client is not None:
                    client.close()
        return DailyReportExecutionResult(
            completion=JobCompletion(status=JobStatus.SUCCEEDED),
            report_id=report.id,
            report_version=report.version,
        )


def _uuid(row: DatabaseRow, key: str) -> UUID:
    value = row[key]
    return value if isinstance(value, UUID) else UUID(str(value))


def _optional_uuid(row: DatabaseRow, key: str) -> UUID | None:
    return None if row[key] is None else _uuid(row, key)


def _datetime(row: DatabaseRow, key: str) -> datetime:
    value = row[key]
    if not isinstance(value, datetime) or value.tzinfo is None:
        raise RuntimeError(f"{key} has an invalid database value")
    return value


def _optional_datetime(row: DatabaseRow, key: str) -> datetime | None:
    return None if row[key] is None else _datetime(row, key)


def _int(row: DatabaseRow, key: str) -> int:
    value = row[key]
    if isinstance(value, bool) or not isinstance(value, int):
        raise RuntimeError(f"{key} has an invalid database value")
    return cast(int, value)


def _optional_str(row: DatabaseRow, key: str) -> str | None:
    value = row[key]
    if value is None:
        return None
    if not isinstance(value, str):
        raise RuntimeError(f"{key} has an invalid database value")
    return value


def _optional_bool(row: DatabaseRow, key: str) -> bool | None:
    value = row[key]
    if value is None or isinstance(value, bool):
        return value
    raise RuntimeError(f"{key} has an invalid database value")


def _metrics(row: DatabaseRow) -> ReportMetricInput:
    return ReportMetricInput(
        like_count=int(row["like_count"] or 0),
        comment_count=int(row["comment_count"] or 0),
        repost_count=int(row["repost_count"] or 0),
        view_count=int(row["view_count"] or 0),
        play_count=int(row["play_count"] or 0),
        danmaku_count=int(row["danmaku_count"] or 0),
    )
