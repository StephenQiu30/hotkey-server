from __future__ import annotations

from apps.api.app.models.hotspot import Hotspot
from apps.api.app.models.keyword import Keyword

from .base import BaseLLMProvider, LLMResult
from .registry import register_llm_provider


@register_llm_provider("fallback")
class FallbackLLMProvider(BaseLLMProvider):
    provider_name = "fallback"

    def expand_queries(self, keyword: Keyword, base_query: str) -> list[str]:
        suffixes = [f"{keyword.keyword} AI", f"{keyword.keyword} news", f"{keyword.keyword} launch", f"{keyword.keyword} update"]
        return self.normalize_queries([base_query, *suffixes])[:5]

    def analyze(self, hotspot: Hotspot, keyword: Keyword | None) -> LLMResult:
        text = f"{hotspot.title} {hotspot.snippet or ''}".lower()
        keyword_text = (keyword.keyword if keyword else "").lower()
        mentioned = bool(keyword_text and keyword_text in text)
        score = 80.0 if mentioned else 45.0
        importance = "high" if score >= 80 else "medium" if score >= 50 else "low"
        return LLMResult(
            is_real=True,
            relevance_score=score,
            relevance_reason="本地降级分析：根据标题和摘要中是否包含关键词判断相关性。",
            keyword_mentioned=mentioned,
            importance=importance,
            summary=hotspot.snippet or hotspot.title,
            model_name="local-fallback",
            raw_response={"provider": "fallback"},
            used_fallback=True,
            prompt_name="fallback",
            token_usage=None,
            provider="fallback",
        )
