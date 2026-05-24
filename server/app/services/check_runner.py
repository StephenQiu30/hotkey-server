from __future__ import annotations

from datetime import datetime, timezone
from urllib.parse import urlparse, urlencode, parse_qsl, urlunparse
from uuid import uuid5, NAMESPACE_URL

from sqlalchemy import Integer, cast, func, select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from server.app.core.settings import settings
from server.app.models.ai_analysis import AiAnalysis
from server.app.models.check_run import CheckRun
from server.app.models.hotspot import Hotspot
from server.app.models.keyword import Keyword
from server.app.models.source import Source
from server.app.services.ai_analysis import analyze_hotspot, expand_keyword_queries, is_analysis_active
from server.app.services.ingestion import SourceIngestionError, fetch_candidates
from server.app.services.notification import notify_hotspot
from server.app.services.hotspot_scoring import HotnessDecision, compute_hotness_score
from server.app.services.providers.selector import select_sources
from server.app.services.source_trust import SourceEvidence, collect_source_evidence


def run_hotspot_check(session: Session, trigger_type: str = "manual") -> CheckRun:
    ensure_default_sources(session)
    check_run = CheckRun(trigger_type=trigger_type, status="running")
    session.add(check_run)
    session.flush()

    errors: list[str] = []
    success_count = 0
    failure_count = 0
    keywords = list(session.scalars(select(Keyword).where(Keyword.enabled.is_(True)).order_by(Keyword.priority.desc(), Keyword.id)))
    sources = select_sources(list(session.scalars(select(Source).where(Source.enabled.is_(True)).order_by(Source.id))))
    url_source_hits: dict[str, set[int]] = {}

    if not keywords:
        errors.append("No enabled keywords.")
        failure_count += 1
    if not sources:
        errors.append("No enabled sources.")
        failure_count += 1

    seen_urls: set[tuple[int, str]] = set()

    for source in sources:
        for keyword in keywords:
            for query in expand_keyword_queries(keyword):
                try:
                    candidates = fetch_candidates(source, keyword, query=query, record_health=True)
                except SourceIngestionError as exc:
                    failure_count += 1
                    errors.append(str(exc))
                    continue
                for candidate in candidates:
                    candidate.url = _normalize_url(candidate.url)
                    if not candidate.url:
                        continue
                    seen_key = (source.id, candidate.url)
                    if seen_key in seen_urls:
                        continue
                    seen_urls.add(seen_key)
                    cross_source_count = _estimate_cross_sources(session, candidate.url, source.id, url_source_hits)
                    candidate.raw_payload.setdefault("cross_source_count", cross_source_count)
                    cluster_id = _cluster_id(candidate)
                    candidate.raw_payload["cluster_id"] = cluster_id
                    candidate.raw_payload["cluster_version"] = _next_cluster_version(session, cluster_id)
                    candidate.raw_payload["clustered_at"] = datetime.now(timezone.utc).isoformat()
                    try:
                        hotspot = _get_or_create_hotspot(session, candidate=candidate)
                    except IntegrityError:
                        continue
                    if hotspot is None:
                        continue
                    evidence = collect_source_evidence(hotspot, cross_source_count=cross_source_count)
                    analysis_result = analyze_hotspot(
                        hotspot,
                        keyword,
                        prefer_langgraph=_should_enhance_analysis(evidence, settings.AI_USE_LANGGRAPH),
                    )
                    hotness = _build_hotness_decision(hotspot=hotspot, analysis=analysis_result, evidence=evidence)
                    _append_enrichment_payload(candidate.raw_payload, analysis_result=analysis_result, evidence=evidence, hotness=hotness)
                    hotspot.status = _decide_hotspot_status(analysis_result, hotness)
                    hotspot.raw_payload.update(candidate.raw_payload)
                    analysis = AiAnalysis(
                        hotspot_id=hotspot.id,
                        is_real=analysis_result.is_real,
                        relevance_score=analysis_result.relevance_score,
                        relevance_reason=analysis_result.relevance_reason,
                        keyword_mentioned=analysis_result.keyword_mentioned,
                        importance=analysis_result.importance,
                        summary=analysis_result.summary,
                        model_name=analysis_result.model_name,
                        raw_response=analysis_result.raw_response,
                    )
                    session.add(analysis)
                    session.flush()
                    if hotspot.status == "active":
                        notification = notify_hotspot(session, hotspot, analysis)
                        if notification.status == "failed":
                            failure_count += 1
                            errors.append(f"Notification failed for hotspot {hotspot.id}.")
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

    params = []
    for key, value in parse_qsl(parsed.query, keep_blank_values=True):
        key_lower = key.lower()
        if key_lower.startswith("utm_") or key_lower in {"fbclid", "yclid", "gclid", "ref", "referer", "source"}:
            continue
        params.append((key_lower, value))
    cleaned_query = urlencode(sorted(params))
    netloc = parsed.netloc.lower()
    if netloc.endswith(":80"):
        netloc = netloc[:-3]
    if netloc.endswith(":443"):
        netloc = netloc[:-4]
    scheme = parsed.scheme.lower() or "https"
    if not netloc and source.startswith("//"):
        # Preserve protocol-relative URLs.
        return source
    path = (parsed.path or "/").rstrip("/").lower() or "/"
    return urlunparse((scheme, netloc, path, parsed.params, cleaned_query, ""))


def _cluster_id(candidate) -> str:
    title = (candidate.title or "").strip().lower()
    normalized = " ".join(title.split()[:12]) if title else "untitled"
    return str(uuid5(NAMESPACE_URL, normalized))


def _estimate_cross_sources(session: Session, normalized_url: str, source_id: int, url_source_hits: dict[str, set[int]]) -> int:
    raw_sources = session.scalars(
        select(func.distinct(Hotspot.source_id)).where(Hotspot.url == normalized_url)
    )
    if hasattr(raw_sources, "all"):
        existing_sources = set(raw_sources.all())
    else:
        existing_sources = set(raw_sources)
    seen_sources = url_source_hits.setdefault(normalized_url, set())
    seen_sources.add(source_id)
    return len(existing_sources | seen_sources)


def _build_hotness_decision(
    *,
    hotspot: Hotspot,
    analysis: object,
    evidence: SourceEvidence,
) -> HotnessDecision:
    return compute_hotness_score(hotspot=hotspot, analysis=analysis, trust_penalty=evidence.penalty())


def _append_enrichment_payload(
    payload: dict[str, object],
    *,
    analysis_result: object,
    evidence: SourceEvidence,
    hotness: HotnessDecision,
) -> None:
    raw_bundle = evidence.bundle()
    payload.update(
        {
            "provider": getattr(analysis_result, "provider", "unknown"),
            "quick_understanding": list(getattr(analysis_result, "quick_understanding", [])),
            "topic_ideas": list(getattr(analysis_result, "topic_ideas", [])),
            "token_usage": getattr(analysis_result, "token_usage", None),
            "prompt_name": getattr(analysis_result, "prompt_name", None),
            "provider_trace": list(getattr(analysis_result, "provider_trace", [])),
            **hotness.raw_payload(),
            "truth_score": evidence.truth_score(),
            "source_risk_level": evidence.risk_level(),
            "source_risk_tags": evidence.risk_tags,
            "source_evidence_bundle": raw_bundle,
            "source_evidence_version": int(raw_bundle.get("version", 0)),
        }
    )


def _decide_hotspot_status(result: object, hotness: HotnessDecision) -> str:
    return "active" if is_analysis_active(result) and hotness.score >= settings.hotness_active_threshold else "filtered"


def _should_enhance_analysis(evidence: SourceEvidence, *, langgraph_enabled: bool) -> bool:
    if not langgraph_enabled:
        return False
    return evidence.truth_score() <= settings.ai_enhance_risk_threshold


def _next_cluster_version(session: Session, cluster_id: str) -> int:
    if not cluster_id:
        return 1
    max_version = session.scalar(
        select(
            func.coalesce(
                func.max(cast(Hotspot.raw_payload["cluster_version"], Integer)),
                0,
            )
        ).where(Hotspot.raw_payload["cluster_id"].as_string() == cluster_id)
    )
    return (int(max_version) if max_version is not None else 0) + 1
