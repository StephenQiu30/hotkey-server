# ruff: noqa: RUF001
from __future__ import annotations

from datetime import datetime
from zoneinfo import ZoneInfo

from reports.schemas import (
    DailyReportData,
    ReportSentiment,
    SourceCoverageStatus,
)

_REPORT_TIMEZONE = ZoneInfo("Asia/Shanghai")
_SENTIMENT_LABELS = {
    ReportSentiment.POSITIVE: "正面",
    ReportSentiment.NEUTRAL: "中性",
    ReportSentiment.NEGATIVE: "负面",
}
_COVERAGE_LABELS = {
    SourceCoverageStatus.SUCCEEDED: "成功",
    SourceCoverageStatus.PARTIAL: "部分成功",
    SourceCoverageStatus.FAILED: "失败",
    SourceCoverageStatus.INCOMPLETE: "进行中",
    SourceCoverageStatus.MISSING: "无采集记录",
}


def _local_minute(value: datetime) -> str:
    return value.astimezone(_REPORT_TIMEZONE).strftime("%Y-%m-%d %H:%M")


def _inline(value: str) -> str:
    normalized = " ".join(value.split())
    for character in ("\\", "`", "*", "_", "[", "]", "<", ">"):
        normalized = normalized.replace(character, f"\\{character}")
    return normalized


def _link(label: str, url: str | None) -> str:
    safe_label = _inline(label)
    return f"[{safe_label}](<{url}>)" if url is not None else safe_label


def _comparison(delta: int) -> str:
    if delta > 0:
        return f"+{delta}"
    if delta < 0:
        return str(delta)
    return "持平"


def _distribution(values: dict[str, int]) -> str:
    if not values:
        return "无"
    return "、".join(f"{key} {values[key]}" for key in sorted(values))


def render_daily_report(data: DailyReportData) -> str:
    """Render the frozen structured report without consulting mutable state."""
    window_label = (
        f"> 数据窗口：{_local_minute(data.window_start)}—"
        f"{_local_minute(data.window_end)}（Asia/Shanghai）"
    )
    posts_comparison = _comparison(data.overview.posts.delta)
    comments_comparison = _comparison(data.overview.comments.delta)
    lines = [
        f"# {_inline(data.topic_name)} 日报",
        "",
        window_label,
        f"> 截止时间：{_local_minute(data.cutoff_at)}（Asia/Shanghai）",
        "> 生成方式：模板版",
        "",
        "## 今日概览",
        "",
        f"- 相关帖子：{data.overview.posts.current}（较前一日 {posts_comparison}）",
        f"- 评论：{data.overview.comments.current}（较前一日 {comments_comparison}）",
        f"- 平台分布：{_distribution(data.overview.platform_distribution)}",
        "- 情感分布："
        + "、".join(
            f"{_SENTIMENT_LABELS[sentiment]} {data.overview.sentiment_distribution[sentiment]}"
            for sentiment in (
                ReportSentiment.POSITIVE,
                ReportSentiment.NEUTRAL,
                ReportSentiment.NEGATIVE,
            )
        ),
        "",
        "## 重点内容 Top 10",
        "",
    ]
    if not data.top_contents:
        lines.append("- 本时间窗暂无已分析的相关内容")
    for index, item in enumerate(data.top_contents, start=1):
        lines.extend(
            (
                f"{index}. [{item.citation}] {_link(item.title, item.url)}",
                f"   - 平台：{item.source_key}；"
                f"情感：{_SENTIMENT_LABELS[item.sentiment]}；"
                f"互动量：{item.interaction_count}",
                f"   - 摘要：{_inline(item.summary)}",
            )
        )
        if item.representative_comments:
            lines.append("   - 代表评论：")
            lines.extend(
                f"     - {_inline(comment.text)}" for comment in item.representative_comments
            )

    lines.extend(("", "## 风险提示", ""))
    if data.risks:
        lines.extend(
            f"- [{item.citation}] {_link(item.title, item.url)}："
            f"{_inline(item.reason)}（互动量 {item.interaction_count}）"
            for item in data.risks
        )
    else:
        lines.append("- 本时间窗暂无负面高互动内容")

    lines.extend(("", "## 值得关注的声音", ""))
    if data.voices:
        lines.extend(
            f"- [{item.citation}] “{_inline(item.excerpt)}” — {_link('原帖', item.url)}"
            for item in data.voices
        )
    else:
        lines.append("- 本时间窗暂无可展示的代表声音")

    lines.extend(("", "## 数据覆盖说明", ""))
    for source in data.coverage.sources:
        lines.append(
            f"- {source.source_key}：{_COVERAGE_LABELS[source.status]}"
            f"（成功 {source.succeeded_jobs}，部分成功 {source.partial_jobs}，"
            f"失败 {source.failed_jobs}，进行中 {source.incomplete_jobs}）"
        )
    if not data.coverage.sources:
        lines.append("- 来源采集：无已配置来源")
    lines.extend(
        (
            f"- 按发现时间计入：{data.coverage.discovered_at_count} 条",
            f"- 未分析：{data.coverage.unanalyzed_count} 条",
            f"- {data.coverage.disclaimer}",
            "",
        )
    )
    return "\n".join(lines)
