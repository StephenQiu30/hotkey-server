from __future__ import annotations

import asyncio
import json
from typing import Any

import httpx2
import pytest

from hotkey_agent.config import Settings
from hotkey_agent.contracts import AnalyzeRequest, Evidence
from hotkey_agent.model_runtime import (
    ModelCompletion,
    ModelOutputInvalidError,
    ModelRateLimitedError,
    ModelRuntimeError,
    ModelTimeoutError,
    OpenAICompatibleAnalyzer,
    OpenAICompatibleClient,
    analyzer_from_settings,
)

TOKEN = "test-agent-token-0123456789abcdef0123456789abcdef"
HASH = "c" * 64


def _settings(**overrides: object) -> Settings:
    values: dict[str, object] = {
        "auth_token": TOKEN,
        "runtime": "openai_compatible",
        "max_request_bytes": 262_144,
        "max_concurrency": 2,
        "model_base_url": "https://models.example.test/v1",
        "model_api_key": "model-api-key-0123456789abcdef",
        "model_name": "trusted-analysis-model",
        "model_version": "trusted-model-2026-08-28",
        "model_timeout_seconds": 30,
        "model_max_response_bytes": 1_048_576,
        "model_max_output_tokens": 4_096,
    }
    values.update(overrides)
    return Settings(**values)  # type: ignore[arg-type]


def _request() -> AnalyzeRequest:
    return AnalyzeRequest(
        contract_version="analysis.v1",
        task_id="shadow-relevance-1",
        task_type="relevance",
        input_hash=HASH,
        evidence_set_hash=HASH,
        payload={
            "schema_name": "relevance-review-output-v1",
            "schema_version": "v1",
            "instruction": "Return the bounded relevance review.",
            "input_schema": {"type": "object"},
            "schema": {
                "type": "object",
                "additionalProperties": False,
                "required": ["decision", "score", "reason_codes"],
                "properties": {
                    "decision": {"type": "string"},
                    "score": {"type": "number"},
                    "reason_codes": {"type": "array", "items": {"type": "string"}},
                },
            },
            "input": {"content_excerpt": "Synthetic evidence only."},
        },
        evidence=[Evidence(id="evidence-1", title="Synthetic", text="Synthetic evidence only.")],
    )


def test_openai_compatible_settings_fail_closed_until_every_secret_and_bound_is_valid() -> None:
    assert _settings().ready is True
    for settings in (
        _settings(model_api_key=""),
        _settings(model_api_key="model-api-key-0123456789abcdef\n"),
        _settings(model_api_key=" model-api-key-0123456789abcdef"),
        _settings(model_base_url="http://models.example.test/v1"),
        _settings(model_base_url="https://user:pass@models.example.test/v1"),
        _settings(model_base_url="https://models.example.test/v1?secret=value"),
        _settings(model_name=""),
        _settings(model_version=""),
        _settings(model_timeout_seconds=0),
        _settings(model_max_response_bytes=8_388_609),
        _settings(model_max_output_tokens=0),
        _settings(runtime="unknown"),
    ):
        assert settings.ready is False

    assert _settings(model_base_url="https://models.example.test:invalid/v1").model_ready is False
    assert analyzer_from_settings(_settings()) is not None
    assert analyzer_from_settings(_settings(runtime="deterministic")) is None
    with pytest.raises(ValueError):
        OpenAICompatibleClient(_settings(model_api_key=""))
    with pytest.raises(ValueError):
        OpenAICompatibleAnalyzer(_settings(model_api_key=""))


def test_model_settings_from_environment_preserve_bounds_and_fail_closed(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    values = {
        "HOTKEY_AGENT_AUTH_TOKEN": TOKEN,
        "HOTKEY_AGENT_RUNTIME": "openai_compatible",
        "HOTKEY_AGENT_MAX_REQUEST_BYTES": "1024",
        "HOTKEY_AGENT_MAX_CONCURRENCY": "4",
        "HOTKEY_AGENT_MODEL_BASE_URL": "https://models.example.test/v1",
        "HOTKEY_AGENT_MODEL_API_KEY": "model-api-key-0123456789abcdef",
        "HOTKEY_AGENT_MODEL_NAME": "trusted-analysis-model",
        "HOTKEY_AGENT_MODEL_VERSION": "trusted-model-2026-08-28",
        "HOTKEY_AGENT_MODEL_TIMEOUT_SECONDS": "60",
        "HOTKEY_AGENT_MODEL_MAX_RESPONSE_BYTES": "2048",
        "HOTKEY_AGENT_MODEL_MAX_OUTPUT_TOKENS": "512",
    }
    for name, value in values.items():
        monkeypatch.setenv(name, value)
    settings = Settings.from_env()
    assert settings.ready is True
    assert settings.max_request_bytes == 1024
    assert settings.max_concurrency == 4
    assert settings.model_timeout_seconds == 60
    assert settings.model_max_response_bytes == 2048
    assert settings.model_max_output_tokens == 512

    monkeypatch.setenv("HOTKEY_AGENT_MAX_CONCURRENCY", "not-an-int")
    monkeypatch.setenv("HOTKEY_AGENT_MODEL_TIMEOUT_SECONDS", "not-an-int")
    monkeypatch.setenv("HOTKEY_AGENT_MODEL_MAX_RESPONSE_BYTES", "0")
    monkeypatch.setenv("HOTKEY_AGENT_MODEL_MAX_OUTPUT_TOKENS", "999999")
    invalid = Settings.from_env()
    assert invalid.max_concurrency == 2
    assert invalid.model_timeout_seconds == 0
    assert invalid.model_max_response_bytes == 0
    assert invalid.model_max_output_tokens == 0
    assert invalid.ready is False


def test_openai_compatible_client_sends_only_bounded_structured_context_and_reads_usage() -> None:
    settings = _settings()

    async def handler(request: httpx2.Request) -> httpx2.Response:
        assert str(request.url) == "https://models.example.test/v1/chat/completions"
        assert request.headers["Authorization"] == f"Bearer {settings.model_api_key}"
        payload = json.loads(request.content)
        assert payload["model"] == settings.model_name
        assert payload["temperature"] == 0
        assert payload["max_tokens"] == settings.model_max_output_tokens
        assert payload["response_format"]["json_schema"]["strict"] is True
        encoded_messages = json.dumps(payload["messages"], ensure_ascii=False)
        assert "Synthetic evidence only." in encoded_messages
        assert settings.auth_token not in encoded_messages
        assert settings.model_api_key not in encoded_messages
        return httpx2.Response(
            200,
            json={
                "model": settings.model_version,
                "choices": [
                    {
                        "message": {
                            "content": json.dumps(
                                {
                                    "decision": "review",
                                    "score": 0,
                                    "reason_codes": ["insufficient_evidence"],
                                }
                            )
                        }
                    }
                ],
                "usage": {"prompt_tokens": 17, "completion_tokens": 9},
            },
        )

    client = OpenAICompatibleClient(settings, transport=httpx2.MockTransport(handler))
    request = _request()
    completion = asyncio.run(
        client.complete(
            schema_name=str(request.payload["schema_name"]),
            instruction=str(request.payload["instruction"]),
            input_value=request.payload["input"],
            output_schema=request.payload["schema"],
            repair=None,
        )
    )
    assert completion == ModelCompletion(
        value={
            "decision": "review",
            "score": 0,
            "reason_codes": ["insufficient_evidence"],
        },
        model_version=settings.model_version,
        input_tokens=17,
        output_tokens=9,
    )


def test_openai_compatible_analyzer_returns_non_degraded_structured_result_with_usage() -> None:
    settings = _settings()

    class ClientFake:
        async def complete(self, **_arguments: Any) -> ModelCompletion:
            return ModelCompletion(
                value={
                    "decision": "review",
                    "score": 0,
                    "reason_codes": ["insufficient_evidence"],
                },
                model_version=settings.model_version,
                input_tokens=17,
                output_tokens=9,
            )

    response = asyncio.run(
        OpenAICompatibleAnalyzer(settings, client=ClientFake()).analyze(_request())
    )
    assert response.status == "succeeded"
    assert response.runtime.name == "openai_compatible"
    assert response.runtime.version == settings.model_version
    assert response.runtime.degraded is False
    assert response.usage is not None
    assert response.usage.input_tokens == 17
    assert response.usage.output_tokens == 9
    assert response.suggestions[0].evidence_ids == ["evidence-1"]
    assert response.suggestions[0].confidence == 0


def test_openai_compatible_client_redacts_provider_failures_and_rejects_model_drift() -> None:
    settings = _settings()

    async def rate_limited(_request: httpx2.Request) -> httpx2.Response:
        return httpx2.Response(429, text="secret=provider-key prompt=private")

    client = OpenAICompatibleClient(settings, transport=httpx2.MockTransport(rate_limited))
    with pytest.raises(ModelRateLimitedError) as captured:
        asyncio.run(
            client.complete(
                schema_name="relevance-review-output-v1",
                instruction="Return JSON.",
                input_value={},
                output_schema={"type": "object"},
                repair=None,
            )
        )
    assert "provider-key" not in str(captured.value)
    assert "private" not in str(captured.value)

    async def drifted(_request: httpx2.Request) -> httpx2.Response:
        return httpx2.Response(
            200,
            json={
                "model": "unapproved-model-version",
                "choices": [{"message": {"content": "{}"}}],
                "usage": {"prompt_tokens": 1, "completion_tokens": 1},
            },
        )

    drifted_client = OpenAICompatibleClient(settings, transport=httpx2.MockTransport(drifted))
    with pytest.raises(ModelOutputInvalidError):
        asyncio.run(
            drifted_client.complete(
                schema_name="relevance-review-output-v1",
                instruction="Return JSON.",
                input_value={},
                output_schema={"type": "object"},
                repair=None,
            )
        )


def test_openai_compatible_client_maps_network_status_size_and_shape_failures() -> None:
    settings = _settings()

    class OversizedBody(httpx2.AsyncByteStream):
        chunks_yielded = 0

        async def __aiter__(self):  # type: ignore[no-untyped-def]
            for chunk in (b"x" * 40, b"x" * 40, b"must-not-be-consumed"):
                self.chunks_yielded += 1
                yield chunk

    oversized_body = OversizedBody()

    async def complete_with(client: OpenAICompatibleClient) -> ModelCompletion:
        return await client.complete(
            schema_name="relevance-review-output-v1",
            instruction="Return JSON.",
            input_value={},
            output_schema={"type": "object"},
            repair={"violations": ["retry"]},
        )

    async def timeout_handler(request: httpx2.Request) -> httpx2.Response:
        raise httpx2.ReadTimeout("timeout with secret", request=request)

    async def connect_handler(request: httpx2.Request) -> httpx2.Response:
        raise httpx2.ConnectError("connect with secret", request=request)

    async def gateway_timeout(_request: httpx2.Request) -> httpx2.Response:
        return httpx2.Response(504)

    async def unavailable(_request: httpx2.Request) -> httpx2.Response:
        return httpx2.Response(503, text="provider secret")

    async def oversized(_request: httpx2.Request) -> httpx2.Response:
        return httpx2.Response(200, stream=oversized_body)

    async def malformed(_request: httpx2.Request) -> httpx2.Response:
        return httpx2.Response(200, json={"model": settings.model_version})

    async def invalid_usage(_request: httpx2.Request) -> httpx2.Response:
        return httpx2.Response(
            200,
            json={
                "model": settings.model_version,
                "choices": [{"message": {"content": "{}"}}],
                "usage": {"prompt_tokens": True, "completion_tokens": 1},
            },
        )

    cases = (
        (timeout_handler, settings, ModelTimeoutError),
        (connect_handler, settings, ModelRuntimeError),
        (gateway_timeout, settings, ModelTimeoutError),
        (unavailable, settings, ModelRuntimeError),
        (oversized, _settings(model_max_response_bytes=64), ModelOutputInvalidError),
        (malformed, settings, ModelOutputInvalidError),
        (invalid_usage, settings, ModelOutputInvalidError),
    )
    for handler, case_settings, error_type in cases:
        client = OpenAICompatibleClient(case_settings, transport=httpx2.MockTransport(handler))
        with pytest.raises(error_type) as captured:
            asyncio.run(complete_with(client))
        assert "secret" not in str(captured.value)
    assert oversized_body.chunks_yielded == 2
