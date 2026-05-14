from __future__ import annotations

from datetime import datetime, timezone
from urllib.parse import urlparse, urlencode, parse_qsl, urlunparse
from uuid import uuid5, NAMESPACE_URL

from sqlalchemy import select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from apps.api.app.core.settings import settings
from apps.api.app.models.ai_analysis import AiAnalysis
from apps.api.app.models.check_run import CheckRun
from apps.api.app.models.hotspot import Hotspot
from apps.api.app.models.keyword import Keyword
from apps.api.app.models.source import Source
from apps.api.app.services.ai_analysis import analyze_hotspot, expand_keyword_queries, is_analysis_active
from apps.api.app.services.ingestion import SourceIngestionError, fetch_candidates
from apps.api.app.services.notification import notify_hotspot


def run_hotspot_check(session: Session, trigger_type: str = "manual") -> CheckRun:
    ensure_default_sources(session)
    check_run = CheckRun(trigger_type=trigger_type, status="running")
    session.add(check_run)
    session.flush()

    errors: list[str] = []
    success_count = 0
    failure_count = 0
    keywords = list(session.scalars(select(Keyword).where(Keyword.enabled.is_(True)).order_by(Keyword.priority.desc(), Keyword.id)))
    sources = list(session.scalars(select(Source).where(Source.enabled.is_(True)).order_by(Source.id)))

    if not keywords:
        errors.append("No enabled keywords.")
    if not sources:
        errors.append("No enabled sources.")

    seen_urls: set[tuple[int, str]] = set()

    for source in sources:
        for keyword in keywords:
            for query in expand_keyword_queries(keyword):
                try:
                    candidates = fetch_candidates(source, keyword, query=query)
                except SourceIngestionError as exc:
                    failure_count += 1
                    errors.append(str(exc))
                    continue
                for candidate in candidates:
                    candidate.url = _normalize_url(candidate.url)
                    if not candidate.url:
                        continue
                    seen_key = (source.id, candidate.url.lower())
                    if seen_key in seen_urls:
                        continue
                    seen_urls.add(seen_key)
                    candidate.raw_payload["cluster_id"] = _cluster_id(candidate)
                    try:
                        hotspot = _get_or_create_hotspot(session, candidate=candidate)
                    except IntegrityError:
                        continue
                    if hotspot is None:
                        continue
                    analysis_result = analyze_hotspot(hotspot, keyword)
                    hotspot.status = "active" if is_analysis_active(analysis_result) else "filtered"
                    analysis = AiAnalysis(
                        hotspot_id=hotspot.id,
                        is_real=analysis_result.is_real,
                        relevance_score=analysis_result.relevance_score,
                        relevance_reason=analysis_result.relevance_reason,
                        keyword_mentioned=analysis_result.keyword_mentioned,
                        importance=analysis_result.importance,
                        summary=analysis_result.summary,
                        model_name=analysis_result.model_name,
                        raw_response={
                            "provider": analysis_result.provider,
                            **analysis_result.raw_response,
                            "token_usage": analysis_result.token_usage,
                            "prompt_name": analysis_result.prompt_name,
                        },
                    )
                    session.add(analysis)
                    session.flush()
                    if hotspot.status == "active":
                        notify_hotspot(session, hotspot, analysis)
                    success_count += 1
                    if analysis_result.used_fallback:
                        errors.append(f"AI fallback used for hotspot {hotspot.id}.")

    check_run.status = "completed" if failure_count == 0 else "completed_with_errors"
    check_run.success_count = success_count
    check_run.failure_count = failure_count
    check_run.error_summary = "\n".join(errors[:20]) if errors else None
    check_run.finished_at = datetime.now(timezone.utc)
    session.commit()
    session.refresh(check_run)
    return check_run


def ensure_default_sources(session: Session) -> None:
    existing_names = set(session.scalars(select(Source.name)))
    defaults = [
        Source(name="Default RSS", source_type="rss", enabled=True, config={"url": "https://hnrss.org/frontpage", "limit": settings.source_fetch_limit}),
        Source(name="Hacker News", source_type="hacker_news", enabled=True, config={"limit": settings.source_fetch_limit}),
        Source(name="X/Twitter", source_type="x_twitter", enabled=False, config={"limit": settings.source_fetch_limit}),
        Source(name="Bing", source_type="bing", enabled=False, config={"limit": settings.source_fetch_limit, "mkt": "zh-CN"}),
        Source(name="Bilibili", source_type="bilibili", enabled=False, config={"limit": settings.source_fetch_limit}),
        Source(name="Sogou", source_type="sogou", enabled=False, config={"limit": settings.source_fetch_limit}),
    ]
    for source in defaults:
        if source.name not in existing_names:
            session.add(source)
    session.flush()


def _get_or_create_hotspot(session: Session, candidate) -> Hotspot | None:
    existing = session.scalar(select(Hotspot).where(Hotspot.source_id == candidate.source_id, Hotspot.url == candidate.url))
    if existing:
        return None
    hotspot = Hotspot(
        title=candidate.title,
        url=candidate.url,
        source_id=candidate.source_id,
        keyword_id=candidate.keyword_id,
        author=candidate.author,
        snippet=candidate.snippet,
        published_at=candidate.published_at,
        raw_payload=candidate.raw_payload,
    )
    try:
        session.add(hotspot)
        session.flush()
    except IntegrityError:
        session.rollback()
        return None
    return hotspot


def _normalize_url(url: str) -> str:
    source = (url or "").strip()
    if not source:
        return ""
    parsed = urlparse(source)
    if not parsed.scheme and not parsed.netloc:
        return source
    if parsed.scheme and not parsed.netloc and parsed.scheme not in {"http", "https"}:
        return source

    params = [(key, value) for key, value in parse_qsl(parsed.query, keep_blank_values=True) if not key.startswith("utm_")]
    cleaned_query = urlencode(params)
    netloc = parsed.netloc.lower()
    scheme = parsed.scheme.lower() or "https"
    if not netloc and source.startswith("//"):
        # Preserve protocol-relative URLs.
        return source
    path = parsed.path.rstrip("/") or "/"
    return urlunparse((scheme, netloc, path, parsed.params, cleaned_query, ""))


def _cluster_id(candidate) -> str:
    title = (candidate.title or "").strip().lower()
    normalized = " ".join(title.split()[:12]) if title else "untitled"
    return str(uuid5(NAMESPACE_URL, normalized))
