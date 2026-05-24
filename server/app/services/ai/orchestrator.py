from __future__ import annotations

import time
import uuid
from dataclasses import dataclass
from typing import Any

from server.app.core.settings import settings as app_settings
from server.app.models.hotspot import Hotspot
from server.app.models.keyword import Keyword
from server.app.services.ai.providers import BaseLLMProvider
from server.app.services.ai.providers.base import LLMResult


@dataclass(slots=True)
class AIPathDecision:
    path: str
    provider: str
    trace_id: str
    decision: dict[str, Any]


def _next_trace_id() -> str:
    return str(uuid.uuid4())


class AIOrchestrator:
    """Default LangChain-first orchestrator for analysis, query expansion and checks."""

    def __init__(self, provider: BaseLLMProvider) -> None:
        self.provider = provider

    def _record(self, decision: AIPathDecision, event: str, **fields: object) -> dict[str, object]:
        decision.decision.setdefault("provider_trace", []).append({"event": event, "trace_id": decision.trace_id, **fields})
        decision.decision["ai_orchestrator_decision"] = decision.path
        decision.decision["trace_id"] = decision.trace_id
        decision.decision.setdefault("enhance_path", "enhanced" if decision.path == "langgraph" else "default")
        return decision.decision

    def analyze(self, hotspot: Hotspot, keyword: Keyword | None) -> tuple[LLMResult, AIPathDecision]:
        start = time.perf_counter()
        trace_id = _next_trace_id()
        decision = AIPathDecision(
            path="langchain",
            provider=self.provider.provider_name,
            trace_id=trace_id,
            decision={"path": "langchain", "provider": self.provider.provider_name},
        )
        result = self.provider.analyze(hotspot, keyword)
        duration = (time.perf_counter() - start) * 1000
        self._record(decision, "analyze_success", duration_ms=duration)
        result.raw_response = dict(result.raw_response)
        result.raw_response["provider_trace"] = decision.decision.get("provider_trace", [])
        result.raw_response["ai_orchestrator_decision"] = decision.decision.get("ai_orchestrator_decision")
        result.raw_response["enhance_path"] = decision.decision.get("enhance_path")
        result.raw_response["trace_id"] = decision.trace_id
        return result, decision

    def expand_queries(self, keyword: Keyword, base_query: str) -> tuple[list[str], AIPathDecision]:
        trace_id = _next_trace_id()
        decision = AIPathDecision(
            path="langchain",
            provider=self.provider.provider_name,
            trace_id=trace_id,
            decision={"path": "langchain", "provider": self.provider.provider_name},
        )
        queries = self.provider.expand_queries(keyword, base_query)
        self._record(decision, "expand_success")
        return queries, decision

    def fact_check_basic(self, hotspot: Hotspot) -> tuple[bool, AIPathDecision]:
        trace_id = _next_trace_id()
        decision = AIPathDecision(
            path="langchain",
            provider=self.provider.provider_name,
            trace_id=trace_id,
            decision={"path": "langchain", "provider": self.provider.provider_name},
        )
        title = (hotspot.title or "").lower()
        snippet = (hotspot.snippet or "").lower()
        is_suspicious = "谣言" in title or "诈骗" in snippet or "未证实" in snippet
        self._record(decision, "fact_check_basic", suspicious=is_suspicious)
        return is_suspicious, decision


class LangGraphOrchestrator(AIOrchestrator):
    """Opt-in enhanced path. Falls back to LangChain when enhanced path fails."""

    def __init__(self, provider: BaseLLMProvider) -> None:
        super().__init__(provider)

    def analyze(self, hotspot: Hotspot, keyword: Keyword | None) -> tuple[LLMResult, AIPathDecision]:
        start = time.perf_counter()
        trace_id = _next_trace_id()
        decision = AIPathDecision(
            path="langgraph",
            provider=self.provider.provider_name,
            trace_id=trace_id,
            decision={"path": "langgraph", "provider": self.provider.provider_name},
        )
        try:
            result = self.provider.analyze(hotspot, keyword)
            duration = (time.perf_counter() - start) * 1000
            timeout_ms = float(app_settings.ai_langgraph_timeout_seconds) * 1000
            if timeout_ms > 0 and duration > timeout_ms:
                raise TimeoutError(f"LangGraph analyze timeout: {duration:.1f}ms > {timeout_ms:.0f}ms")
            self._record(decision, "analyze_success", duration_ms=duration)
            result.raw_response = dict(result.raw_response)
            result.raw_response["provider_trace"] = decision.decision.get("provider_trace", [])
            result.raw_response["ai_orchestrator_decision"] = decision.decision.get("ai_orchestrator_decision")
            result.raw_response["enhance_path"] = decision.decision.get("enhance_path")
            result.raw_response["trace_id"] = decision.trace_id
            return result, decision
        except Exception as exc:  # noqa: BLE001
            fallback_orchestrator = AIOrchestrator(self.provider)
            fallback_result, fallback_decision = fallback_orchestrator.analyze(hotspot, keyword)
            self._record(decision, "langgraph_analyze_fallback", error=str(exc), fallback_to="langchain")
            fallback_decision.decision["path_from"] = "langgraph"
            fallback_decision.decision["langgraph_fallback"] = True
            fallback_decision.decision["langgraph_path_status"] = "fallbacked"
            fallback_decision.decision["fallback_reason"] = str(exc)
            fallback_decision.decision.setdefault("enhance_path", "fallback")
            fallback_traces = fallback_decision.decision.get("provider_trace")
            if isinstance(fallback_traces, list):
                fallback_traces.extend(decision.decision.get("provider_trace", []))
            return fallback_result, fallback_decision

    def expand_queries(self, keyword: Keyword, base_query: str) -> tuple[list[str], AIPathDecision]:
        try:
            queries, decision = super().expand_queries(keyword, base_query)
            decision.path = "langgraph"
            decision.decision["path"] = "langgraph"
            return queries, decision
        except Exception as exc:  # noqa: BLE001
            fallback_queries, decision = super().expand_queries(keyword, base_query)
            decision.decision["langgraph_fallback"] = True
            decision.decision["error"] = str(exc)
            return fallback_queries, decision


def build_orchestrator(provider: BaseLLMProvider, *, use_langgraph: bool = False) -> AIOrchestrator:
    """Build orchestrator by explicit caller request and feature flag."""
    if use_langgraph and app_settings.ai_use_langgraph:
        return LangGraphOrchestrator(provider)
    return AIOrchestrator(provider)
