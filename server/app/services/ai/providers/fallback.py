from __future__ import annotations

from server.app.models.hotspot import Hotspot
from server.app.models.keyword import Keyword

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
        summary = hotspot.snippet or hotspot.title
        topic_keyword = keyword.keyword if keyword else "热点"
        quick_understanding = [
            f"{topic_keyword}相关热点：{hotspot.title}",
            f"相关性评分 {score:.0f}/100，可用于判断是否跟进。",
            f"核心信息：{summary}",
        ]
        topic_ideas = [
            {
                "title": f"3分钟看懂：{hotspot.title}",
                "angle": "快速解释热点背景、变化和受众影响。",
                "format": "短视频/图文",
                "rationale": "适合内容创作者把热点转成快速理解型内容。",
            },
            {
                "title": f"{topic_keyword}为什么值得关注",
                "angle": "拆解趋势信号、受益人群和后续观察点。",
                "format": "长图文/直播提纲",
                "rationale": "适合做观点型或深度解读型选题。",
            },
        ]
        return LLMResult(
            is_real=True,
            relevance_score=score,
            relevance_reason="本地降级分析：根据标题和摘要中是否包含关键词判断相关性。",
            keyword_mentioned=mentioned,
            importance=importance,
            summary=summary,
            model_name="local-fallback",
            raw_response={
                "provider": "fallback",
                "quick_understanding": quick_understanding,
                "topic_ideas": topic_ideas,
            },
            quick_understanding=quick_understanding,
            topic_ideas=topic_ideas,
            used_fallback=True,
            prompt_name="fallback",
            token_usage=None,
            provider="fallback",
        )
