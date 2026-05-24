from __future__ import annotations

from datetime import date, datetime, time, timedelta, timezone
from typing import Literal
import html

from sqlalchemy import case, select
from sqlalchemy.orm import Session, selectinload

from server.app.models.ai_analysis import AiAnalysis
from server.app.models.hotspot import Hotspot
from server.app.models.notification import Notification
from server.app.models.report import Report
from server.app.services.notification import notify_report

ReportType = Literal["daily", "weekly"]

REPORT_LIMITS = {"daily": 10, "weekly": 20}


def generate_report(
    session: Session,
    report_type: ReportType = "daily",
    period_start: date | None = None,
) -> Report:
    start, end = report_period(report_type, period_start=period_start)
    hotspots = _load_report_hotspots(session, start, end, limit=REPORT_LIMITS[report_type])
    subject, summary, content = _render_report(report_type, start, end, hotspots)

    report = session.scalar(
        select(Report).where(
            Report.report_type == report_type,
            Report.period_start == start,
            Report.period_end == end,
        )
    )
    if report is None:
        report = Report(
            report_type=report_type,
            period_start=start,
            period_end=end,
            subject=subject,
            summary=summary,
            content=content,
        )
        session.add(report)
    else:
        report.subject = subject
        report.summary = summary
        report.content = content
        report.status = "generated"
        report.sent_at = None
    report.hotspot_count = len(hotspots)
    session.flush()
    session.refresh(report)
    return report


def send_report(session: Session, report: Report) -> Notification:
    notification = notify_report(session, report)
    if notification.status == "sent":
        report.status = "sent"
        report.sent_at = notification.sent_at
    elif notification.status == "failed":
        report.status = "failed"
    else:
        report.status = "skipped"
    session.flush()
    session.refresh(report)
    return notification


def generate_and_send_report(
    session: Session,
    report_type: ReportType = "daily",
    period_start: date | None = None,
) -> Report:
    report = generate_report(session, report_type=report_type, period_start=period_start)
    send_report(session, report)
    return report


def report_period(
    report_type: ReportType,
    period_start: date | None = None,
    *,
    now: datetime | None = None,
) -> tuple[datetime, datetime]:
    today = (now or datetime.now(timezone.utc)).date()
    target = period_start or today
    if report_type == "daily":
        start = datetime.combine(target, time.min, tzinfo=timezone.utc)
        return start, start + timedelta(days=1)
    if report_type == "weekly":
        week_start = target - timedelta(days=target.weekday())
        start = datetime.combine(week_start, time.min, tzinfo=timezone.utc)
        return start, start + timedelta(days=7)
    raise ValueError(f"Unsupported report_type: {report_type}")


def previous_daily_period_start(now: datetime | None = None) -> date:
    return (now or datetime.now(timezone.utc)).date() - timedelta(days=1)


def previous_weekly_period_start(now: datetime | None = None) -> date:
    current = (now or datetime.now(timezone.utc)).date()
    this_week_start = current - timedelta(days=current.weekday())
    return this_week_start - timedelta(days=7)


def _load_report_hotspots(session: Session, start: datetime, end: datetime, limit: int) -> list[Hotspot]:
    importance_rank = case(
        (AiAnalysis.importance == "high", 3),
        (AiAnalysis.importance == "medium", 2),
        (AiAnalysis.importance == "low", 1),
        else_=0,
    )
    stmt = (
        select(Hotspot)
        .join(AiAnalysis, AiAnalysis.hotspot_id == Hotspot.id)
        .options(
            selectinload(Hotspot.source),
            selectinload(Hotspot.keyword),
            selectinload(Hotspot.ai_analysis),
        )
        .where(Hotspot.fetched_at >= start, Hotspot.fetched_at < end, Hotspot.status == "active")
        .order_by(importance_rank.desc(), AiAnalysis.relevance_score.desc(), Hotspot.fetched_at.desc())
        .limit(limit)
    )
    return list(session.scalars(stmt).unique())


def _render_report(
    report_type: ReportType,
    start: datetime,
    end: datetime,
    hotspots: list[Hotspot],
) -> tuple[str, str, str]:
    label = "日报" if report_type == "daily" else "周报"
    if report_type == "daily":
        period_label = start.date().isoformat()
    else:
        period_label = f"{start.date().isoformat()} 至 {(end - timedelta(days=1)).date().isoformat()}"
    subject = f"AI 热点{label} - {period_label}"

    if not hotspots:
        summary = f"本期没有发现符合条件的 AI 热点。"
        content = "\n".join([f"# {subject}", "", summary])
        return subject, summary, content

    high_count = sum(1 for hotspot in hotspots if hotspot.ai_analysis and hotspot.ai_analysis.importance == "high")
    summary = f"本期共筛选出 {len(hotspots)} 条 AI 热点，其中高重要性 {high_count} 条。"
    lines = [f"# {subject}", "", summary, "", "## Top 热点"]
    for index, hotspot in enumerate(hotspots, start=1):
        analysis = hotspot.ai_analysis
        source_name = hotspot.source.name if hotspot.source else "Unknown source"
        keyword = hotspot.keyword.keyword if hotspot.keyword else "未关联关键词"
        lines.extend(
            [
                "",
                f"{index}. {hotspot.title}",
                f"   - 来源：{source_name}",
                f"   - 关键词：{keyword}",
                f"   - 重要性：{analysis.importance if analysis else 'unknown'}",
                f"   - 相关性：{analysis.relevance_score if analysis else 0}",
                f"   - 摘要：{analysis.summary if analysis and analysis.summary else hotspot.snippet or ''}",
                f"   - 理由：{analysis.relevance_reason if analysis and analysis.relevance_reason else ''}",
                f"   - 链接：{hotspot.url}",
            ]
        )
    return subject, summary, "\n".join(lines)


def report_to_html(report: Report) -> str:
    lines = ["<html><head><meta charset=\"utf-8\"></head><body>", f"<h1>{html.escape(report.subject)}</h1>"]
    for line in report.content.splitlines():
        if line.startswith("# "):
            lines.append(f"<h2>{html.escape(line[2:])}</h2>")
        elif line.startswith("## "):
            lines.append(f"<h3>{html.escape(line[3:])}</h3>")
        elif line.startswith("   - "):
            lines.append(f"<li>{html.escape(line[5:])}</li>")
        elif line.strip():
            lines.append(f"<p>{html.escape(line)}</p>")
    lines.append(f"<p>热点数量: {report.hotspot_count}</p>")
    lines.append("</body></html>")
    return "\n".join(lines)
