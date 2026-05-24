from __future__ import annotations

import json
from typing import Any

import httpx

from server.app.core.settings import settings
from server.app.models.hotspot import Hotspot
from server.app.models.keyword import Keyword

from .base import BaseLLMProvider, LLMResult
from .registry import register_llm_provider


@register_llm_provider("openai")
@register_llm_provider("deepseek")
@register_llm_provider("gemini")
class OpenAICompatibleProvider(BaseLLMProvider):
    provider_name = "openai-compatible"

    def _api_key_and_url(self, provider_key: str) -> tuple[str, str, str]:
        provider_key = (provider_key or settings.ai_provider).strip().lower()
        if provider_key == "deepseek":
            return settings.deepseek_api_key or "", settings.deepseek_base_url, settings.deepseek_model or "deepseek-chat"
        if provider_key == "gemini":
            return settings.gemini_api_key or "", settings.gemini_base_url, settings.gemini_model or "gemini-pro"
        return settings.openai_api_key or "", settings.openai_base_url, settings.openai_model or "gpt-4o-mini"

    def _base_url(self, provider_key: str | None = None) -> str:
        if provider_key is None:
            provider_key = settings.ai_provider
        provider_key = (provider_key or settings.ai_provider).strip().lower()
        return (self._api_key_and_url(provider_key)[1] or "https://api.openai.com/v1").rstrip("/")

    def expand_queries(self, keyword: Keyword, base_query: str) -> list[str]:
        response = self._chat("expand", keyword, base_query)
        parsed = self._parse_model_json(response.get("content", "{}"))
        raw_queries = [str(item).strip() for item in parsed.get("queries", []) if str(item).strip()]
        return self.normalize_queries([base_query, *raw_queries])[:5]

    def analyze(self, hotspot: Hotspot, keyword: Keyword | None) -> LLMResult:
        provider_key = settings.ai_provider
        response = self._chat(
            prompt_type="analysis",
            keyword=keyword,
            title=hotspot.title,
            snippet=hotspot.snippet or "",
            url=hotspot.url,
            provider_key=provider_key,
        )
        parsed = self._parse_model_json(response.get("content", "{}"))
        quick_understanding = _string_list(parsed.get("quick_understanding"))
        topic_ideas = _topic_idea_list(parsed.get("topic_ideas"))
        return LLMResult(
            is_real=parsed.get("is_real"),
            relevance_score=self.clamp_score(float(parsed.get("relevance_score", 0))),
            relevance_reason=str(parsed.get("relevance_reason") or ""),
            keyword_mentioned=bool(parsed.get("keyword_mentioned")),
            importance=str(parsed.get("importance") or "medium"),
            summary=str(parsed.get("summary") or hotspot.snippet or hotspot.title),
            model_name=parsed.get("model", self._resolve_model(provider_key)),
            raw_response=response,
            quick_understanding=quick_understanding,
            topic_ideas=topic_ideas,
            used_fallback=False,
            prompt_name=response.get("prompt_name", "analysis"),
            token_usage=dict(
                prompt_tokens=response.get("usage", {}).get("prompt_tokens", 0),
                completion_tokens=response.get("usage", {}).get("completion_tokens", 0),
                total_tokens=response.get("usage", {}).get("total_tokens", 0),
            ),
            provider=provider_key,
        )

    def _chat(
        self,
        prompt_type: str,
        keyword: Keyword | None = None,
        title: str = "",
        snippet: str = "",
        url: str = "",
        provider_key: str | None = None,
    ) -> dict[str, Any]:
        prompts = self.build_prompt_pack()
        api_key = self._resolve_key(provider_key)
        model = self._resolve_model(provider_key)
        base_url = self._base_url(provider_key)
        if not api_key:
            raise RuntimeError("LLM API key is not configured.")
        if prompt_type == "expand":
            user_prompt = (
                "Return strict JSON with key queries as an array of 2 to 5 short search queries. "
                f"Keyword: {keyword.keyword if keyword else ''}\\nTemplate: {title}"
            )
        else:
            user_prompt = (
                "Analyze this hotspot candidate. Return strict JSON with keys: "
                "is_real, relevance_score, relevance_reason, keyword_mentioned, importance, summary, "
                "quick_understanding, topic_ideas, model. "
                "importance must be low, medium, or high. relevance_score is 0-100. "
                "summary, relevance_reason and quick_understanding must be written in Chinese. "
                "quick_understanding is 2 to 4 short strings. "
                "topic_ideas is 2 to 5 objects with title, angle, format and rationale in Chinese.\n\n"
                f"Keyword: {keyword.keyword if keyword else ''}\n"
                f"Title: {title}\n"
                f"Snippet: {snippet}\n"
                f"URL: {url}"
            )
        system_prompt = prompts["expand" if prompt_type == "expand" else "analysis"]
        payload = {
            "model": model,
            "messages": [
                {"role": "system", "content": system_prompt},
                {"role": "user", "content": user_prompt},
            ],
            "temperature": 0.2 if prompt_type == "expand" else 0.1,
        }
        headers = {"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"}
        response = httpx.post(f"{base_url}/chat/completions", headers=headers, json=payload, timeout=30)
        response.raise_for_status()
        data = response.json()
        choices = data.get("choices", [])
        message = choices[0].get("message", {}) if choices else {}
        content = message.get("content", "{}")
        return {
            "model": data.get("model", model),
            "content": content,
            "usage": data.get("usage", {}),
            "prompt_name": prompt_type,
        }

    def _resolve_key(self, provider_key: str | None = None) -> str:
        provider = (provider_key or settings.ai_provider).strip().lower()
        if provider == "deepseek":
            return settings.deepseek_api_key or ""
        if provider == "gemini":
            return settings.gemini_api_key or ""
        return settings.openai_api_key or ""

    def _resolve_model(self, provider_key: str | None = None) -> str:
        provider = (provider_key or settings.ai_provider).strip().lower()
        if provider == "deepseek":
            return settings.deepseek_model or "deepseek-chat"
        if provider == "gemini":
            return settings.gemini_model or "gemini-pro"
        return settings.openai_model or "gpt-4o-mini"

    @staticmethod
    def _parse_model_json(content: str) -> dict[str, Any]:
        content = content.strip()
        try:
            return json.loads(content)
        except json.JSONDecodeError:
            start = content.find("{")
            end = content.rfind("}")
            if start < 0 or end < 0 or end <= start:
                raise
            return json.loads(content[start : end + 1])


def _string_list(value: object) -> list[str]:
    if not isinstance(value, list):
        return []
    return [str(item).strip() for item in value if str(item).strip()]


def _topic_idea_list(value: object) -> list[dict[str, str]]:
    if not isinstance(value, list):
        return []
    ideas: list[dict[str, str]] = []
    for item in value:
        if not isinstance(item, dict):
            continue
        title = str(item.get("title") or "").strip()
        angle = str(item.get("angle") or "").strip()
        format_name = str(item.get("format") or "").strip()
        rationale = str(item.get("rationale") or "").strip()
        if not title:
            continue
        ideas.append(
            {
                "title": title,
                "angle": angle,
                "format": format_name,
                "rationale": rationale,
            }
        )
    return ideas
