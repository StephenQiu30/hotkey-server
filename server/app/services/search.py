from __future__ import annotations

from datetime import datetime, timezone
from sqlalchemy import func, select
from sqlalchemy.orm import Session

from server.app.core.settings import settings
from server.app.models.hotspot import Hotspot
from server.app.models.keyword import Keyword
from server.app.models.source import Source
from server.app.schemas.search import SearchRead, SearchResultRead
from server.app.services.ai_analysis import analyze_hotspot, expand_keyword_queries, is_analysis_active
from server.app.services.check_runner import ensure_default_sources
from server.app.services.ingestion import SourceIngestionError, fetch_candidates
from server.app.services.hotspot_scoring import HotnessDecision, compute_hotness_score
from server.app.services.providers import normalize_source_type
from server.app.services.providers.selector import select_sources
from server.app.services.source_trust import SourceEvidence, collect_source_evidence


def search_sources(session: Session, query: str, source_types: list[str] | None = None, limit: int = 20) -> SearchRead:
    ensure_default_sources(session)
    keyword = Keyword(keyword=query, query_template=query, enabled=True)
    sources = _load_search_sources(session, source_types)
    errors: list[str] = []
    items: list[SearchResultRead] = []

    if not sources:
        errors.append("No enabled sources matched the search request.")
        return SearchRead(query=query, items=[], errors=errors)

    for source in sources:
        for expanded_query in expand_keyword_queries(keyword):
            try:
                candidates = fetch_candidates(source, keyword, query=expanded_query)
            except SourceIngestionError as exc:
                errors.append(str(exc))
                continue
            for candidate in candidates:
                normalized_url = _normalize_url_for_search(candidate.url)
                if not normalized_url:
                    continue
                cross_source_count = _count_cross_sources(session, normalized_url)
                hotspot = Hotspot(
                    title=candidate.title,
                    url=normalized_url,
                    source_id=source.id,
                    keyword_id=None,
                    author=candidate.author,
                    snippet=candidate.snippet,
                    published_at=candidate.published_at,
                    raw_payload=candidate.raw_payload,
                )
                hotspot.source = source
                evidence = collect_source_evidence(hotspot, cross_source_count=cross_source_count)
                analysis = analyze_hotspot(hotspot, keyword, prefer_langgraph=False)
                hotness = _build_hotness_decision(hotspot=hotspot, analysis=analysis, evidence=evidence)
                if _should_enhance_analysis(
                    evidence,
                    hotness_score=hotness.score,
                    langgraph_enabled=settings.ai_use_langgraph,
                ):
                    analysis = analyze_hotspot(hotspot, keyword, prefer_langgraph=True)
                    hotness = _build_hotness_decision(hotspot=hotspot, analysis=analysis, evidence=evidence)
                _append_enrichment_payload(candidate.raw_payload, analysis_result=analysis, evidence=evidence, hotness=hotness)
                status = "filtered" if _is_low_trust_blocked(evidence) else (
                    "active" if is_analysis_active(analysis) and hotness.score >= settings.hotness_active_threshold else "filtered"
                )
                items.append(
                    SearchResultRead(
                        title=candidate.title,
                        url=normalized_url,
                        source_id=source.id,
                        source_name=source.name,
                        source_type=source.source_type,
                        author=candidate.author,
                        published_at=candidate.published_at,
                        snippet=candidate.snippet,
                        relevance_score=analysis.relevance_score,
                        relevance_reason=analysis.relevance_reason,
                        keyword_mentioned=analysis.keyword_mentioned,
                        importance=analysis.importance,
                        summary=analysis.summary,
                        status=status,
                        hotness_score=float(hotness.score),
                        hotness_version=hotness.version,
                        hotness_reason=hotness.reason,
                        source_risk_level=evidence.risk_level(),
                        source_risk_tags=evidence.risk_tags,
                        source_evidence_bundle=evidence.bundle(),
                        raw_payload=candidate.raw_payload,
                    )
                )
                if len(items) >= limit:
                    return SearchRead(query=query, items=_sort_items(items), errors=errors)
    return SearchRead(query=query, items=_sort_items(items)[:limit], errors=errors)


def _load_search_sources(session: Session, source_types: list[str] | None) -> list[Source]:
    stmt = select(Source).where(Source.enabled.is_(True)).order_by(Source.id)
    if source_types:
        normalized = [normalize_source_type(source_type) for source_type in source_types]
        stmt = stmt.where(Source.source_type.in_(normalized))
    return select_sources(list(session.scalars(stmt)))


def _sort_items(items: list[SearchResultRead]) -> list[SearchResultRead]:
    importance_rank = {"high": 3, "medium": 2, "low": 1}
    return sorted(
        items,
        key=lambda item: (
            item.hotness_score,
            item.relevance_score,
            item.published_at or datetime.min.replace(tzinfo=timezone.utc),
            importance_rank.get(item.importance, 0),
        ),
        reverse=True,
    )


def _count_cross_sources(session: Session, normalized_url: str) -> int:
    statement = select(func.count(func.distinct(Hotspot.source_id))).where(Hotspot.url == normalized_url)
    scalar_fn = getattr(session, "scalar", None)
    if callable(scalar_fn):
        total = scalar_fn(statement)
        return int(total or 0) + 1

    raw_scalars = getattr(session, "scalars", None)
    if not callable(raw_scalars):
        return 1

    raw_total = raw_scalars(statement)
    if hasattr(raw_total, "all"):
        rows = raw_total.all()
    else:
        rows = raw_total
    return int(rows[0] if rows else 0) + 1


def _normalize_url_for_search(url: str) -> str:
    return (url or "").strip()


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
            **hotness.raw_payload(),
            "provider": getattr(analysis_result, "provider", "unknown"),
            "quick_understanding": list(getattr(analysis_result, "quick_understanding", [])),
            "topic_ideas": list(getattr(analysis_result, "topic_ideas", [])),
            "token_usage": getattr(analysis_result, "token_usage", None),
            "prompt_name": getattr(analysis_result, "prompt_name", None),
            "provider_trace": list(getattr(analysis_result, "provider_trace", [])),
            "ai_orchestrator_decision": getattr(analysis_result, "ai_orchestrator_decision", None),
            "enhance_path": getattr(analysis_result, "enhance_path", "default"),
            "fallback_reason": getattr(analysis_result, "fallback_reason", None),
            "trace_id": getattr(analysis_result, "trace_id", None),
            "truth_score": evidence.truth_score(),
            "source_risk_level": evidence.risk_level(),
            "source_risk_tags": evidence.risk_tags,
            "source_evidence_bundle": raw_bundle,
            "source_evidence_version": int(raw_bundle.get("version", 0)),
            "cross_source_count": int(getattr(evidence, "cross_source_count", 1)),
        }
    )


def _should_enhance_analysis(
    evidence: SourceEvidence,
    *,
    hotness_score: float,
    langgraph_enabled: object,
) -> bool:
    # Mirror check-runner gating: environment values like "false" must keep LangGraph disabled.
    if not _is_langgraph_enabled(langgraph_enabled):
        return False
    source_conflict = getattr(evidence, "cross_source_count", 1) >= 2
    truth_low = evidence.truth_score() <= settings.ai_enhance_risk_threshold
    return hotness_score >= settings.ai_enhance_hotness_threshold and (source_conflict or truth_low)


def _is_langgraph_enabled(value: object) -> bool:
    if isinstance(value, str):
        return value.strip().lower() in {"1", "true", "yes", "on"}
    return bool(value)


def _is_low_trust_blocked(evidence: SourceEvidence) -> bool:
    return evidence.risk_level() == "low"
