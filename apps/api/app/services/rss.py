from __future__ import annotations

from datetime import datetime
from xml.etree.ElementTree import Element, SubElement, tostring

from sqlalchemy import select
from sqlalchemy.orm import Session

from apps.api.app.models.hotspot import Hotspot
from apps.api.app.models.keyword import Keyword

from apps.api.app.models.ai_analysis import AiAnalysis
from apps.api.app.models.report import Report


def generate_trending_feed(session: Session, limit: int = 50) -> str:
    hotspots = _load_top_hotspots(session, limit=limit)
    return _render_feed("AI热点趋势", "热点趋势", hotspots)


def generate_keyword_feed(session: Session, keyword_name: str, limit: int = 50) -> str:
    keyword = session.scalar(select(Keyword).where(Keyword.keyword == keyword_name))
    if keyword is None:
        return _render_feed(f"关键词: {keyword_name}", f"关键词 {keyword_name} 未匹配到记录", [])
    stmt = (
        select(Hotspot)
        .where(Hotspot.keyword_id == keyword.id, Hotspot.status == "active")
        .order_by(Hotspot.fetched_at.desc())
        .limit(limit)
    )
    return _render_feed(f"关键词: {keyword.keyword}", f"{keyword.keyword} 关键词热点", list(session.scalars(stmt)))


def generate_ai_summary_feed(session: Session, limit: int = 20) -> str:
    report = session.scalar(select(Report).order_by(Report.period_start.desc(), Report.id.desc()).limit(1))
    if report is None:
        return _render_feed("AI 摘要", "暂无 AI 摘要", [])
    analysis = [
        {
            "title": report.subject,
            "url": f"/api/reports/{report.id}",
            "published_at": report.created_at,
            "summary": report.summary or "",
            "analysis": "AI日报",
            "source": "report",
            "raw": report.content,
        }
    ]
    return _render_feed("AI 摘要", report.summary or "AI日报", analysis)


def _load_top_hotspots(session: Session, limit: int) -> list[Hotspot]:
    stmt = (
        select(Hotspot)
        .join(AiAnalysis, AiAnalysis.hotspot_id == Hotspot.id)
        .where(Hotspot.status == "active")
        .order_by(AiAnalysis.relevance_score.desc(), Hotspot.fetched_at.desc())
        .limit(limit)
    )
    return list(session.scalars(stmt))


def _render_feed(title: str, description: str, items: list[Hotspot] | list[dict[str, object]]) -> str:
    root = Element("rss", attrib={"version": "2.0"})
    channel = SubElement(root, "channel")
    SubElement(channel, "title").text = title
    SubElement(channel, "description").text = description
    SubElement(channel, "link").text = "http://localhost"

    for index, item in enumerate(items):
        if isinstance(item, dict):
            _append_item_from_dict(channel, item, index)
        else:
            _append_item_from_hotspot(channel, item, index)
    return tostring(root, encoding="utf-8", xml_declaration=True).decode("utf-8")


def _append_item_from_hotspot(channel: Element, item: Hotspot, index: int) -> None:
    entry = SubElement(channel, "item")
    SubElement(entry, "title").text = item.title
    SubElement(entry, "link").text = item.url
    SubElement(entry, "guid").text = str(item.id or index)
    if item.published_at:
        SubElement(entry, "pubDate").text = item.published_at.isoformat()
    snippet = item.snippet or ""
    analysis = item.ai_analysis
    summary = analysis.summary if analysis else snippet
    heat = str(analysis.relevance_score) if analysis else "0"
    source_name = item.source.name if getattr(item, "source", None) else "unknown"
    SubElement(entry, "description").text = summary
    SubElement(entry, "category").text = f"hotness={heat} source={source_name}".strip()


def _append_item_from_dict(channel: Element, item: dict[str, str | datetime], index: int) -> None:
    entry = SubElement(channel, "item")
    SubElement(entry, "title").text = str(item.get("title", "AI摘要"))
    SubElement(entry, "link").text = str(item.get("url", ""))
    SubElement(entry, "guid").text = str(item.get("source", "report") + str(index))
    published = item.get("published_at")
    if isinstance(published, datetime):
        SubElement(entry, "pubDate").text = published.isoformat()
    SubElement(entry, "description").text = str(item.get("summary", ""))
    SubElement(entry, "category").text = str(item.get("analysis", ""))
