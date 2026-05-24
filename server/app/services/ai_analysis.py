from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import Any

from server.app.core.settings import settings
from server.app.models.hotspot import Hotspot
from server.app.models.keyword import Keyword
from server.app.services.ai.providers import BaseLLMProvider, LLMResult, build_provider
from server.app.services.ai.orchestrator import build_orchestrator


logger = logging.getLogger("ai_hotspot_radar")


@dataclass(slots=True)
class AnalysisResult:
    is_real: bool | None
    relevance_score: float
    relevance_reason: str
    keyword_mentioned: bool
    importance: str
    summary: str
    model_name: str
    raw_response: dict[str, Any]
    quick_understanding: list[str] = field(default_factory=list)
    topic_ideas: list[dict[str, str]] = field(default_factory=list)
    used_fallback: bool = False
    prompt_name: str | None = None
    token_usage: dict[str, int] | None = None
    provider_trace: list[dict[str, Any]] = field(default_factory=list)
    provider: str = ""


def _normalize_strategy(strategy: str | None) -> str:
    normalized = (strategy or "").strip().lower()
    if normalized in {"fallback", "skip", "error"}:
        return normalized
    return "fallback"


def _normalize_provider_key(value: str | None) -> str:
    return (value or settings.ai_fallback_provider or "fallback").strip().lower()


def _safe_build_provider(provider_key: str) -> BaseLLMProvider:
    return build_provider(provider_key or "fallback")


def _select_provider() -> BaseLLMProvider:
    configured = settings.ai_provider.strip().lower() if settings.ai_provider else "fallback"
    try:
        logger.info("ai_provider_selection", extra={"provider": configured})
        return _safe_build_provider(configured)
    except Exception:
        logger.warning(
            "ai_provider_selection_fallback",
            extra={"provider": configured, "fallback": _normalize_provider_key(settings.ai_fallback_provider)},
        )
        try:
            return _safe_build_provider(_normalize_provider_key(settings.ai_fallback_provider))
        except Exception:
            logger.exception("ai_provider_selection_fallback_failed", extra={"provider": _normalize_provider_key(settings.ai_fallback_provider)})
            return _safe_build_provider("fallback")


def _build_skip_analysis(hotspot: Hotspot, keyword: Keyword | None, error: Exception) -> AnalysisResult:
    logger.warning(
        "ai_provider_skip",
        extra={
            "strategy": _normalize_strategy(settings.ai_provider_error_strategy),
            "primary_provider": settings.ai_provider,
            "error": str(error),
            "hotspot_id": hotspot.id,
        },
    )
    return AnalysisResult(
        is_real=None,
        relevance_score=0.0,
        relevance_reason=f"AI analysis skipped due to provider error: {str(error)}",
        keyword_mentioned=False,
        importance="low",
        summary=hotspot.snippet or hotspot.title,
        model_name=_normalize_provider_key(settings.ai_fallback_provider),
        raw_response={"provider": "skipped", "reason": str(error)},
        quick_understanding=[f"AI 分析被策略跳过，已保留摘要：{(hotspot.title[:40])}"],
        topic_ideas=[
            {
                "title": "待人工补充热点分析",
                "angle": "模型异常下建议人工复核热点真实性与相关性。",
                "format": "人工复核清单",
                "rationale": "自动化链路暂时未生成结构化结果。",
            }
        ],
        used_fallback=False,
        prompt_name="skipped",
        provider_trace=[{"event": "analysis_skipped", "reason": str(error)}],
        token_usage=None,
        provider="skipped",
    )


def _apply_error_strategy(exc: Exception, hotspot: Hotspot, keyword: Keyword | None) -> AnalysisResult:
    strategy = _normalize_strategy(settings.ai_provider_error_strategy)
    if strategy == "skip":
        return _build_skip_analysis(hotspot, keyword, exc)
    if strategy == "error":
        raise

    logger.warning(
        "ai_provider_fallback",
        extra={
            "strategy": strategy,
            "primary_provider": settings.ai_provider,
            "fallback_provider": _normalize_provider_key(settings.ai_fallback_provider),
            "error": str(exc),
            "hotspot_id": hotspot.id,
        },
    )
    fallback_provider = _normalize_provider_key(settings.ai_fallback_provider)
    try:
        fallback = _safe_build_provider(fallback_provider)
        fallback_result = _to_analysis_result(fallback.analyze(hotspot, keyword))
        fallback_result.used_fallback = True
        fallback_result.raw_response = {**fallback_result.raw_response, "fallback_reason": str(exc), "fallback_from": settings.ai_provider}
        fallback_result.provider_trace = [
            {
                "event": "provider_fallback",
                "source": settings.ai_provider,
                "target": fallback.provider_name,
                "error": str(exc),
            }
        ]
        fallback_result.provider = fallback.provider_name
        return fallback_result
    except Exception as fallback_exc:
        logger.warning(
            "ai_provider_fallback_failed",
            extra={"primary_provider": settings.ai_provider, "fallback_provider": fallback_provider, "error": str(fallback_exc)},
        )
        return _build_skip_analysis(hotspot, keyword, fallback_exc)




def _analyze_with_provider(
    provider: BaseLLMProvider,
    hotspot: Hotspot,
    keyword: Keyword | None,
    *,
    prefer_langgraph: bool = False,
) -> AnalysisResult:
    orchestrator = build_orchestrator(provider, use_langgraph=prefer_langgraph)
    result, decision = orchestrator.analyze(hotspot, keyword)
    final = _to_analysis_result(result)
    final.raw_response = {
        **final.raw_response,
        "provider_trace": decision.decision.get("provider_trace", []),
    }
    trace_payload = decision.decision.get("provider_trace")
    if isinstance(trace_payload, list):
        final.provider_trace = list(trace_payload)
    return final


def analyze_hotspot(
    hotspot: Hotspot,
    keyword: Keyword | None,
    *,
    prefer_langgraph: bool = False,
) -> AnalysisResult:
    provider = _select_provider()
    try:
        return _analyze_with_provider(
            provider=provider,
            hotspot=hotspot,
            keyword=keyword,
            prefer_langgraph=prefer_langgraph,
        )
    except Exception as exc:  # noqa: BLE001
        if provider.__class__.__name__ == "FallbackLLMProvider":
            raise
        return _apply_error_strategy(exc, hotspot, keyword)


def expand_keyword_queries(keyword: Keyword) -> list[str]:
    provider = _select_provider()
    base_query = keyword.query_template or keyword.keyword
    try:
        return _dedupe_queries(provider.expand_queries(keyword, base_query))[:5]
    except Exception:  # noqa: BLE001
        if _normalize_strategy(settings.ai_provider_error_strategy) == "error":
            raise
        if _normalize_strategy(settings.ai_provider_error_strategy) == "skip":
            logger.warning(
                "ai_query_expand_skip",
                extra={"provider": settings.ai_provider, "keyword": keyword.keyword},
            )
            return [base_query]
        fallback = _fallback_queries(keyword, base_query)
        logger.warning(
            "ai_query_expand_fallback",
            extra={"provider": settings.ai_provider, "fallback_provider": settings.ai_fallback_provider, "keyword": keyword.keyword},
        )
        return fallback


def is_analysis_active(result: AnalysisResult) -> bool:
    return result.relevance_score >= settings.relevance_threshold and result.is_real is not False


def _to_analysis_result(llm_result: LLMResult) -> AnalysisResult:
    usage = llm_result.token_usage
    return AnalysisResult(
        is_real=llm_result.is_real,
        relevance_score=llm_result.relevance_score,
        relevance_reason=llm_result.relevance_reason,
        keyword_mentioned=llm_result.keyword_mentioned,
        importance=llm_result.importance,
        summary=llm_result.summary,
        model_name=llm_result.model_name,
        raw_response=llm_result.raw_response,
        quick_understanding=llm_result.quick_understanding,
        topic_ideas=llm_result.topic_ideas,
        used_fallback=llm_result.used_fallback,
        prompt_name=llm_result.prompt_name,
        token_usage=usage,
        provider_trace=llm_result.provider_trace,
        provider=llm_result.provider,
    )


def _fallback_analysis(hotspot: Hotspot, keyword: Keyword | None) -> AnalysisResult:
    text = f"{hotspot.title} {hotspot.snippet or ''}".lower()
    keyword_text = (keyword.keyword if keyword else "").lower()
    mentioned = bool(keyword_text and keyword_text in text)
    score = 80.0 if mentioned else 45.0
    importance = "high" if score >= 80 else "medium" if score >= 50 else "low"
    summary = hotspot.snippet or hotspot.title
    quick_understanding = _build_quick_understanding(hotspot, keyword, score, summary)
    topic_ideas = _build_topic_ideas(hotspot, keyword)
    return AnalysisResult(
        is_real=True,
        relevance_score=score,
        relevance_reason="本地降级分析：根据标题和摘要中是否包含关键词判断相关性。",
        keyword_mentioned=mentioned,
        importance=importance,
        summary=summary,
        model_name=settings.openai_model or "local-fallback",
        raw_response={
            "provider": "fallback",
            "quick_understanding": quick_understanding,
            "topic_ideas": topic_ideas,
        },
        quick_understanding=quick_understanding,
        topic_ideas=topic_ideas,
        used_fallback=not (settings.openai_api_key and settings.openai_model),
        prompt_name="fallback",
        token_usage=None,
        provider="fallback",
    )


def _build_quick_understanding(hotspot: Hotspot, keyword: Keyword | None, score: float, summary: str) -> list[str]:
    keyword_text = keyword.keyword if keyword else "当前热点"
    return [
        f"{keyword_text}相关热点：{hotspot.title}",
        f"相关性评分 {score:.0f}/100，可优先用于快速理解与选题判断。",
        f"核心信息：{summary}",
    ]


def _build_topic_ideas(hotspot: Hotspot, keyword: Keyword | None) -> list[dict[str, str]]:
    keyword_text = keyword.keyword if keyword else "热点"
    title = hotspot.title.strip()
    return [
        {
            "title": f"3分钟看懂：{title}",
            "angle": "快速解释热点背景、变化和受众影响。",
            "format": "短视频/图文",
            "rationale": "适合内容创作者把热点转成快速理解型内容。",
        },
        {
            "title": f"{keyword_text}为什么值得关注",
            "angle": "拆解趋势信号、受益人群和后续观察点。",
            "format": "长图文/直播提纲",
            "rationale": "适合做观点型或深度解读型选题。",
        },
    ]


def _fallback_queries(keyword: Keyword, base_query: str) -> list[str]:
    return _dedupe_queries(
        [
            base_query,
            f"{keyword.keyword} AI",
            f"{keyword.keyword} news",
            f"{keyword.keyword} launch",
            f"{keyword.keyword} update",
        ]
    )


def _dedupe_queries(queries: list[str]) -> list[str]:
    seen: set[str] = set()
    result: list[str] = []
    for query in queries:
        normalized = query.strip()
        key = normalized.lower()
        if normalized and key not in seen:
            seen.add(key)
            result.append(normalized)
    return result
