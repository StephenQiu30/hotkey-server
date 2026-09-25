# ruff: noqa: RUF001
from datetime import UTC, datetime, timedelta
from uuid import UUID

from reports.render import render_daily_report
from reports.schemas import (
    AnnotationState,
    PreparedDailyReport,
    ReportBuildDataset,
    ReportCommentInput,
    ReportMetricInput,
    ReportPeriod,
    ReportPostInput,
    ReportSourceCoverage,
    SourceCoverageStatus,
)
from reports.services import (
    daily_report_operation_id,
    prepare_daily_report,
    previous_daily_window,
    should_wait_for_report_watermark,
)

OWNER_ID = UUID("10000000-0000-0000-0000-000000000001")
TOPIC_ID = UUID("20000000-0000-0000-0000-000000000001")


def _post(
    suffix: int,
    *,
    period: ReportPeriod,
    source_key: str,
    title: str,
    annotation_state: AnnotationState,
    relevant: bool | None,
    sentiment: str | None,
    summary: str | None,
    reason: str | None,
    like_count: int,
    comment_count: int,
    repost_count: int,
    published: bool = True,
) -> ReportPostInput:
    return ReportPostInput(
        period=period,
        content_id=UUID(f"30000000-0000-0000-0000-{suffix:012d}"),
        content_version_id=UUID(f"40000000-0000-0000-0000-{suffix:012d}"),
        observation_id=UUID(f"50000000-0000-0000-0000-{suffix:012d}"),
        annotation_id=(
            UUID(f"60000000-0000-0000-0000-{suffix:012d}")
            if annotation_state is not AnnotationState.MISSING
            else None
        ),
        source_key=source_key,
        title=title,
        body=f"{title} 的正文",
        url=f"https://example.com/posts/{suffix}",
        published_at=(datetime(2026, 9, 24, 2, suffix, tzinfo=UTC) if published else None),
        first_observed_at=datetime(2026, 9, 24, 2, suffix, tzinfo=UTC),
        occurred_at=datetime(2026, 9, 24, 2, suffix, tzinfo=UTC),
        annotation_state=annotation_state,
        relevant=relevant,
        sentiment=sentiment,
        summary=summary,
        relevance_reason=reason,
        viewpoints=(),
        metrics=ReportMetricInput(
            like_count=like_count,
            comment_count=comment_count,
            repost_count=repost_count,
            view_count=100,
            play_count=0,
            danmaku_count=0,
        ),
    )


def _dataset() -> ReportBuildDataset:
    first = _post(
        1,
        period=ReportPeriod.CURRENT,
        source_key="hackernews",
        title="HotKey 发布新版本",
        annotation_state=AnnotationState.ANNOTATED,
        relevant=True,
        sentiment="negative",
        summary="新版本引发稳定性讨论",
        reason="用户担心升级后的稳定性",
        like_count=10,
        comment_count=2,
        repost_count=3,
        published=False,
    )
    second = _post(
        2,
        period=ReportPeriod.CURRENT,
        source_key="google_news",
        title="团队公布后续计划",
        annotation_state=AnnotationState.ANNOTATED,
        relevant=True,
        sentiment="positive",
        summary="团队公布性能优化计划",
        reason="内容直接讨论产品计划",
        like_count=3,
        comment_count=1,
        repost_count=0,
    )
    irrelevant = _post(
        3,
        period=ReportPeriod.CURRENT,
        source_key="hackernews",
        title="同名项目更新",
        annotation_state=AnnotationState.ANNOTATED,
        relevant=False,
        sentiment=None,
        summary="同名项目，与监控主题无关",
        reason="内容指向同名项目",
        like_count=20,
        comment_count=5,
        repost_count=1,
    )
    unanalyzed = _post(
        4,
        period=ReportPeriod.CURRENT,
        source_key="google_news",
        title="尚未完成分析的内容",
        annotation_state=AnnotationState.MISSING,
        relevant=None,
        sentiment=None,
        summary=None,
        reason=None,
        like_count=1,
        comment_count=0,
        repost_count=0,
    )
    previous = _post(
        5,
        period=ReportPeriod.PREVIOUS,
        source_key="hackernews",
        title="前一日相关内容",
        annotation_state=AnnotationState.ANNOTATED,
        relevant=True,
        sentiment="neutral",
        summary="前一日基准内容",
        reason="内容直接讨论监控主题",
        like_count=1,
        comment_count=1,
        repost_count=0,
    )
    comments = (
        ReportCommentInput(
            period=ReportPeriod.CURRENT,
            post_content_id=first.content_id,
            content_id=UUID("70000000-0000-0000-0000-000000000001"),
            content_version_id=UUID("71000000-0000-0000-0000-000000000001"),
            observation_id=UUID("72000000-0000-0000-0000-000000000001"),
            text="部署后延迟明显下降",
            occurred_at=datetime(2026, 9, 24, 3, tzinfo=UTC),
            metrics=ReportMetricInput(like_count=4),
        ),
        ReportCommentInput(
            period=ReportPeriod.CURRENT,
            post_content_id=first.content_id,
            content_id=UUID("70000000-0000-0000-0000-000000000002"),
            content_version_id=UUID("71000000-0000-0000-0000-000000000002"),
            observation_id=UUID("72000000-0000-0000-0000-000000000002"),
            text="升级文档还需要更清楚",
            occurred_at=datetime(2026, 9, 24, 4, tzinfo=UTC),
            metrics=ReportMetricInput(like_count=2),
        ),
        ReportCommentInput(
            period=ReportPeriod.CURRENT,
            post_content_id=second.content_id,
            content_id=UUID("70000000-0000-0000-0000-000000000003"),
            content_version_id=UUID("71000000-0000-0000-0000-000000000003"),
            observation_id=UUID("72000000-0000-0000-0000-000000000003"),
            text="期待下一轮性能优化",
            occurred_at=datetime(2026, 9, 24, 5, tzinfo=UTC),
            metrics=ReportMetricInput(like_count=1),
        ),
        ReportCommentInput(
            period=ReportPeriod.PREVIOUS,
            post_content_id=previous.content_id,
            content_id=UUID("70000000-0000-0000-0000-000000000004"),
            content_version_id=UUID("71000000-0000-0000-0000-000000000004"),
            observation_id=UUID("72000000-0000-0000-0000-000000000004"),
            text="前一日评论",
            occurred_at=datetime(2026, 9, 23, 5, tzinfo=UTC),
            metrics=ReportMetricInput(),
        ),
    )
    return ReportBuildDataset(
        posts=(first, second, irrelevant, unanalyzed, previous),
        comments=comments,
        source_coverage=(
            ReportSourceCoverage(
                source_key="google_news",
                status=SourceCoverageStatus.PARTIAL,
                succeeded_jobs=1,
                partial_jobs=0,
                failed_jobs=1,
                incomplete_jobs=0,
            ),
            ReportSourceCoverage(
                source_key="hackernews",
                status=SourceCoverageStatus.SUCCEEDED,
                succeeded_jobs=2,
                partial_jobs=0,
                failed_jobs=0,
                incomplete_jobs=0,
            ),
        ),
    )


def _prepare(
    dataset: ReportBuildDataset,
    *,
    previous: PreparedDailyReport | None = None,
) -> PreparedDailyReport:
    return prepare_daily_report(
        owner_id=OWNER_ID,
        topic_id=TOPIC_ID,
        topic_name="HotKey",
        window_start=datetime(2026, 9, 23, 16, tzinfo=UTC),
        window_end=datetime(2026, 9, 24, 16, tzinfo=UTC),
        cutoff_at=datetime(2026, 9, 25, 1, 10, tzinfo=UTC),
        dataset=dataset,
        previous=previous,
    )


def test_fixed_dataset_renders_expected_daily_markdown() -> None:
    prepared = _prepare(_dataset())

    assert (
        render_daily_report(prepared.data)
        == """# HotKey 日报

> 数据窗口：2026-09-24 00:00—2026-09-25 00:00（Asia/Shanghai）
> 截止时间：2026-09-25 09:10（Asia/Shanghai）
> 生成方式：模板版

## 今日概览

- 相关帖子：2（较前一日 +1）
- 评论：3（较前一日 +2）
- 平台分布：google_news 1、hackernews 1
- 情感分布：正面 1、中性 0、负面 1

## 重点内容 Top 10

1. [c1] [HotKey 发布新版本](<https://example.com/posts/1>)
   - 平台：hackernews；情感：负面；互动量：15
   - 摘要：新版本引发稳定性讨论
   - 代表评论：
     - 部署后延迟明显下降
     - 升级文档还需要更清楚
2. [c2] [团队公布后续计划](<https://example.com/posts/2>)
   - 平台：google_news；情感：正面；互动量：4
   - 摘要：团队公布性能优化计划
   - 代表评论：
     - 期待下一轮性能优化

## 风险提示

- [c1] [HotKey 发布新版本](<https://example.com/posts/1>)：用户担心升级后的稳定性（互动量 15）

## 值得关注的声音

- [c1] “部署后延迟明显下降” — [原帖](<https://example.com/posts/1>)
- [c1] “升级文档还需要更清楚” — [原帖](<https://example.com/posts/1>)
- [c2] “期待下一轮性能优化” — [原帖](<https://example.com/posts/2>)

## 数据覆盖说明

- google_news：部分成功（成功 1，部分成功 0，失败 1，进行中 0）
- hackernews：成功（成功 2，部分成功 0，失败 0，进行中 0）
- 按发现时间计入：1 条
- 未分析：1 条
- 样本观察，不代表全网
"""
    )


def test_regeneration_reuses_manifest_and_data_while_incrementing_version() -> None:
    first = _prepare(_dataset())
    changed_dataset = ReportBuildDataset(posts=(), comments=(), source_coverage=())

    second = _prepare(changed_dataset, previous=first)

    assert first.version == 1
    assert second.version == 2
    assert second.cutoff_at == first.cutoff_at
    assert second.input_manifest == first.input_manifest
    assert second.data == first.data
    assert second.body_markdown == first.body_markdown


def test_daily_window_operation_identity_and_watermark_use_shanghai_time() -> None:
    due_at = datetime(2026, 9, 25, 1, tzinfo=UTC)
    window_start, window_end = previous_daily_window(
        due_at,
        report_time=datetime(2026, 9, 25, 9).time(),
    )

    assert window_start == datetime(2026, 9, 23, 16, tzinfo=UTC)
    assert window_end == datetime(2026, 9, 24, 16, tzinfo=UTC)
    assert daily_report_operation_id(
        topic_id=TOPIC_ID,
        window_start=window_start,
    ) == daily_report_operation_id(topic_id=TOPIC_ID, window_start=window_start)
    assert should_wait_for_report_watermark(
        now=due_at + timedelta(minutes=9, seconds=59),
        due_at=due_at,
        unanalyzed_count=1,
    )
    assert not should_wait_for_report_watermark(
        now=due_at + timedelta(minutes=10),
        due_at=due_at,
        unanalyzed_count=1,
    )
