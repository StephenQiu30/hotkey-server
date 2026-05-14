from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from apps.api.app.core.settings import settings
from apps.api.app.models.hotspot import Hotspot
from apps.api.app.models.keyword import Keyword
from apps.api.app.services.ai.providers import BaseLLMProvider, LLMResult, build_provider


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
    used_fallback: bool = False
    prompt_name: str | None = None
    token_usage: dict[str, int] | None = None
    provider: str = ""


def _select_provider() -> BaseLLMProvider:
    configured = settings.ai_provider.strip().lower() if settings.ai_provider else "fallback"
    try:
        return build_provider(configured)
    except Exception:
        return build_provider("fallback")


def analyze_hotspot(hotspot: Hotspot, keyword: Keyword | None) -> AnalysisResult:
    provider = _select_provider()
    try:
        return _to_analysis_result(provider.analyze(hotspot, keyword))
    except Exception as exc:  # noqa: BLE001
        if provider.__class__.__name__ == "FallbackLLMProvider":
            raise
        fallback = _fallback_analysis(hotspot, keyword)
        fallback.raw_response = {"provider": "fallback", "reason": str(exc)}
        fallback.used_fallback = True
        return fallback


def expand_keyword_queries(keyword: Keyword) -> list[str]:
    provider = _select_provider()
    base_query = keyword.query_template or keyword.keyword
    try:
        return _dedupe_queries(provider.expand_queries(keyword, base_query))[:5]
    except Exception:  # noqa: BLE001
        fallback = _fallback_queries(keyword, base_query)
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
        used_fallback=llm_result.used_fallback,
        prompt_name=llm_result.prompt_name,
        token_usage=usage,
        provider=llm_result.provider,
    )


def _fallback_analysis(hotspot: Hotspot, keyword: Keyword | None) -> AnalysisResult:
    text = f"{hotspot.title} {hotspot.snippet or ''}".lower()
    keyword_text = (keyword.keyword if keyword else "").lower()
    mentioned = bool(keyword_text and keyword_text in text)
    score = 80.0 if mentioned else 45.0
    importance = "high" if score >= 80 else "medium" if score >= 50 else "low"
    return AnalysisResult(
        is_real=True,
        relevance_score=score,
        relevance_reason="本地降级分析：根据标题和摘要中是否包含关键词判断相关性。",
        keyword_mentioned=mentioned,
        importance=importance,
        summary=hotspot.snippet or hotspot.title,
        model_name=settings.openai_model or "local-fallback",
        raw_response={"provider": "fallback"},
        used_fallback=not (settings.openai_api_key and settings.openai_model),
        prompt_name="fallback",
        token_usage=None,
        provider="fallback",
    )


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
