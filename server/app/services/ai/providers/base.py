from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any

from server.app.models.hotspot import Hotspot
from server.app.models.keyword import Keyword


@dataclass(slots=True)
class LLMResult:
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
    provider_trace: list[dict[str, Any]] = field(default_factory=list)
    token_usage: dict[str, int] | None = None
    provider: str = ""


@dataclass(slots=True)
class ProviderUsage:
    prompt_tokens: int = 0
    completion_tokens: int = 0
    total_tokens: int = 0


class BaseLLMProvider(ABC):
    """AI provider abstraction used by query expansion and hotspot analysis."""

    provider_name = "base"

    @abstractmethod
    def expand_queries(self, keyword: Keyword, base_query: str) -> list[str]:
        """Expand a keyword query into 2~5 short candidates."""

    @abstractmethod
    def analyze(self, hotspot: Hotspot, keyword: Keyword | None) -> LLMResult:
        """Analyze one hotspot item and return structured scoring summary."""

    @staticmethod
    def normalize_queries(queries: list[str]) -> list[str]:
        seen: set[str] = set()
        normalized: list[str] = []
        for query in queries:
            value = query.strip()
            if not value:
                continue
            key = value.lower()
            if key not in seen:
                seen.add(key)
                normalized.append(value)
        return normalized

    @staticmethod
    def parse_usage(response: dict[str, Any]) -> ProviderUsage | None:
        usage = response.get("usage") if isinstance(response, dict) else None
        if not isinstance(usage, dict):
            return None
        return ProviderUsage(
            prompt_tokens=int(usage.get("prompt_tokens") or 0),
            completion_tokens=int(usage.get("completion_tokens") or 0),
            total_tokens=int(usage.get("total_tokens") or 0),
        )

    @staticmethod
    def build_prompt_pack() -> dict[str, str]:
        return {
            "expand": "You expand hotspot monitoring keywords into concise search queries.",
            "analysis": "You are a precise news relevance analyst.",
        }

    @staticmethod
    def clamp_score(value: float) -> float:
        if value < 0:
            return 0.0
        if value > 100:
            return 100.0
        return float(value)
