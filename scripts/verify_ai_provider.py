from __future__ import annotations

import argparse
import json
from dataclasses import asdict, dataclass, field
from typing import Any

from server.app.core.settings import settings
from server.app.models.hotspot import Hotspot
from server.app.models.keyword import Keyword
from server.app.services.ai_analysis import analyze_hotspot


@dataclass(slots=True)
class ProviderAcceptanceResult:
    ok: bool
    status: str
    provider: str
    model: str | None
    required_env: list[str] = field(default_factory=list)
    trace_id: str | None = None
    ai_orchestrator_decision: str | None = None
    provider_trace: list[dict[str, Any]] = field(default_factory=list)
    validation_errors: list[str] = field(default_factory=list)
    message: str = ""


def _provider_config(provider: str) -> tuple[str | None, str | None, list[str]]:
    normalized = provider.strip().lower()
    if normalized == "deepseek":
        return settings.deepseek_api_key, settings.deepseek_model, ["DEEPSEEK_API_KEY", "DEEPSEEK_MODEL"]
    if normalized == "gemini":
        return settings.gemini_api_key, settings.gemini_model, ["GEMINI_API_KEY", "GEMINI_MODEL"]
    return settings.openai_api_key, settings.openai_model, ["OPENAI_API_KEY", "OPENAI_MODEL"]


def _sample_keyword() -> Keyword:
    return Keyword(id=1, keyword="AI provider 验收", query_template=None, enabled=True, priority=1)


def _sample_hotspot() -> Hotspot:
    return Hotspot(
        id=1,
        title="OpenAI 兼容模型发布新的 AI agent 能力",
        url="https://example.com/hotkey/provider-acceptance",
        source_id=1,
        keyword_id=1,
        author="HotKey Acceptance",
        snippet="用于验证真实外部 AI provider 的 LangChain 默认路径、trace 与 fallback 字段。",
        raw_payload={"trend_score": 90},
        status="new",
    )


def run_provider_acceptance() -> ProviderAcceptanceResult:
    provider = (settings.ai_provider or "openai").strip().lower()
    api_key, model, required_env = _provider_config(provider)
    if not api_key or not model:
        return ProviderAcceptanceResult(
            ok=False,
            status="missing_credentials",
            provider=provider,
            model=model,
            required_env=required_env,
            message="配置真实 provider 凭据后再执行验收；脚本不会输出任何密钥内容。",
        )

    result = analyze_hotspot(_sample_hotspot(), _sample_keyword(), prefer_langgraph=settings.ai_use_langgraph)
    validation_errors: list[str] = []
    if not result.trace_id:
        validation_errors.append("trace_id")
    if result.ai_orchestrator_decision not in {"langchain", "langgraph"}:
        validation_errors.append("ai_orchestrator_decision")
    if not result.provider_trace:
        validation_errors.append("provider_trace")
    if result.relevance_score < 0:
        validation_errors.append("relevance_score")

    if validation_errors:
        return ProviderAcceptanceResult(
            ok=False,
            status="validation_failed",
            provider=provider,
            model=model,
            required_env=required_env,
            trace_id=result.trace_id,
            ai_orchestrator_decision=result.ai_orchestrator_decision,
            provider_trace=result.provider_trace,
            validation_errors=validation_errors,
            message="真实 provider 已返回，但缺少关闭 #50-#53 所需的 trace/decision 证据。",
        )

    return ProviderAcceptanceResult(
        ok=True,
        status="passed",
        provider=provider,
        model=model,
        required_env=required_env,
        trace_id=result.trace_id,
        ai_orchestrator_decision=result.ai_orchestrator_decision,
        provider_trace=result.provider_trace,
        message="真实 provider 验收通过，可将结果回填 #50-#53/#55。",
    )


def main() -> int:
    parser = argparse.ArgumentParser(description="Verify real AI provider acceptance evidence for #50-#53/#55.")
    parser.add_argument("--json", action="store_true", help="Print machine-readable JSON output.")
    args = parser.parse_args()
    result = run_provider_acceptance()
    payload = asdict(result)
    if args.json:
        print(json.dumps(payload, ensure_ascii=False, indent=2))
    else:
        print(f"status={result.status} provider={result.provider} model={result.model or '<missing>'}")
        print(result.message)
        if result.required_env:
            print("required_env=" + ",".join(result.required_env))
        if result.trace_id:
            print(f"trace_id={result.trace_id}")
        if result.ai_orchestrator_decision:
            print(f"ai_orchestrator_decision={result.ai_orchestrator_decision}")
        if result.validation_errors:
            print("validation_errors=" + ",".join(result.validation_errors))
    if result.ok:
        return 0
    if result.status == "missing_credentials":
        return 2
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
