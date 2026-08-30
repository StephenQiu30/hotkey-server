from __future__ import annotations

import asyncio
import json
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

import pytest

from hotkey_agent.config import Settings
from hotkey_agent.contracts import AnalyzeRequest, Evidence
from hotkey_agent.model_runtime import (
    CodexAppServerAnalyzer,
    CodexAppServerClient,
    Connector,
    ModelCompletion,
    ModelOutputInvalidError,
    ModelRateLimitedError,
    ModelRuntimeError,
    ModelTimeoutError,
    _codex_output_schema,
    _completed_value,
    _error_code,
    _strip_optional_nulls,
    _thread_identity,
    _token_usage,
    _turn_identity,
    analyzer_from_settings,
)

TOKEN = "test-agent-token-0123456789abcdef0123456789abcdef"
HASH = "c" * 64


def _settings(**overrides: object) -> Settings:
    values: dict[str, object] = {
        "auth_token": TOKEN,
        "runtime": "codex_app_server",
        "max_request_bytes": 262_144,
        "max_concurrency": 2,
        "codex_app_server_url": "ws://127.0.0.1:4500",
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
                    "score": {"type": "number", "minimum": 0, "maximum": 100},
                    "reason_codes": {
                        "type": "array",
                        "minItems": 1,
                        "uniqueItems": True,
                        "items": {"type": "string"},
                    },
                    "note": {"type": "string", "maxLength": 100},
                },
            },
            "input": {"content_excerpt": "Synthetic evidence only."},
        },
        evidence=[Evidence(id="evidence-1", title="Synthetic", text="Synthetic evidence only.")],
    )


class FakeWebSocket:
    def __init__(self, messages: list[object]) -> None:
        self.messages = list(messages)
        self.sent: list[dict[str, Any]] = []

    async def send(self, message: str) -> None:
        self.sent.append(json.loads(message))

    async def recv(self) -> str | bytes:
        if not self.messages:
            await asyncio.Future()
        value = self.messages.pop(0)
        if isinstance(value, BaseException):
            raise value
        if isinstance(value, (str, bytes)):
            return value
        return json.dumps(value, ensure_ascii=False, separators=(",", ":"))


def _connector(websocket: FakeWebSocket) -> Connector:
    @asynccontextmanager
    async def connect(_url: str, _max_message_bytes: int) -> AsyncIterator[FakeWebSocket]:
        yield websocket

    return connect


def _success_messages(*, item_type: str = "agentMessage") -> list[object]:
    output = json.dumps(
        {"decision": "review", "score": 0, "reason_codes": ["insufficient_evidence"]},
        separators=(",", ":"),
    )
    item: dict[str, object] = {
        "type": item_type,
        "id": "item-1",
        "text": output[:-1] + ',"note":null}',
        "phase": "final_answer",
    }
    if item_type != "agentMessage":
        item = {"type": item_type, "id": "item-1"}
    return [
        {"jsonrpc": "2.0", "id": 1, "result": {"userAgent": "codex-test"}},
        {
            "jsonrpc": "2.0",
            "id": 2,
            "result": {"thread": {"id": "thread-1"}, "model": "gpt-5.6-sol"},
        },
        {"jsonrpc": "2.0", "id": 3, "result": {"turn": {"id": "turn-1"}}},
        {
            "jsonrpc": "2.0",
            "method": "thread/tokenUsage/updated",
            "params": {
                "threadId": "thread-1",
                "turnId": "turn-1",
                "tokenUsage": {
                    "last": {
                        "inputTokens": 17,
                        "cachedInputTokens": 0,
                        "outputTokens": 9,
                        "reasoningOutputTokens": 2,
                        "totalTokens": 26,
                    },
                    "total": {
                        "inputTokens": 17,
                        "cachedInputTokens": 0,
                        "outputTokens": 9,
                        "reasoningOutputTokens": 2,
                        "totalTokens": 26,
                    },
                },
            },
        },
        {
            "jsonrpc": "2.0",
            "method": "turn/completed",
            "params": {
                "threadId": "thread-1",
                "turn": {"id": "turn-1", "status": "completed", "items": [item]},
            },
        },
    ]


async def _complete(client: CodexAppServerClient) -> ModelCompletion:
    request = _request()
    return await client.complete(
        schema_name=str(request.payload["schema_name"]),
        instruction=str(request.payload["instruction"]),
        input_value=request.payload["input"],
        output_schema=request.payload["schema"],
        repair=None,
    )


def test_codex_settings_accept_only_a_local_websocket_and_need_no_model_credentials(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    assert _settings().ready is True
    assert _settings(codex_app_server_url="ws://localhost:4500").ready is True
    assert _settings(codex_app_server_url="ws://[::1]:4500").ready is True
    assert _settings(codex_app_server_url="ws://host.docker.internal:4500").ready is True
    for url in (
        "",
        "http://127.0.0.1:4500",
        "wss://127.0.0.1:4500",
        "ws://192.168.1.10:4500",
        "ws://user:secret@127.0.0.1:4500",
        "ws://127.0.0.1",
        "ws://127.0.0.1:4500/path",
        "ws://127.0.0.1:4500?token=secret",
    ):
        assert _settings(codex_app_server_url=url).ready is False
    assert _settings(runtime="unknown").ready is False
    assert analyzer_from_settings(_settings()) is not None
    assert analyzer_from_settings(_settings(runtime="deterministic")) is None

    monkeypatch.setenv("HOTKEY_AGENT_AUTH_TOKEN", TOKEN)
    monkeypatch.setenv("HOTKEY_AGENT_CODEX_APP_SERVER_URL", "ws://localhost:4600")
    monkeypatch.setenv("HOTKEY_AGENT_RUNTIME", "openai_compatible")
    monkeypatch.setenv("HOTKEY_AGENT_MODEL_API_KEY", "must-be-ignored")
    settings = Settings.from_env()
    assert settings.runtime == "codex_app_server"
    assert settings.codex_app_server_url == "ws://localhost:4600"
    assert settings.ready is True

    monkeypatch.setenv("HOTKEY_AGENT_PREVIOUS_AUTH_TOKENS", "short,short")
    monkeypatch.setenv("HOTKEY_AGENT_MAX_REQUEST_BYTES", "invalid")
    monkeypatch.setenv("HOTKEY_AGENT_MAX_CONCURRENCY", "0")
    invalid = Settings.from_env()
    assert invalid.ready is False
    assert invalid.max_request_bytes == 262_144
    assert invalid.max_concurrency == 2
    assert _settings(codex_app_server_url="ws://127.0.0.1:invalid").ready is False

    with pytest.raises(ValueError):
        CodexAppServerClient(_settings(codex_app_server_url="https://models.example.test"))
    with pytest.raises(ValueError):
        CodexAppServerAnalyzer(_settings(codex_app_server_url="https://models.example.test"))


def test_codex_client_uses_bounded_ephemeral_read_only_protocol_and_reads_usage() -> None:
    messages = _success_messages()
    for message in messages:
        if isinstance(message, dict):
            message.pop("jsonrpc", None)
    websocket = FakeWebSocket(messages)
    completion = asyncio.run(
        _complete(CodexAppServerClient(_settings(), connector=_connector(websocket)))
    )
    assert completion == ModelCompletion(
        value={
            "decision": "review",
            "score": 0,
            "reason_codes": ["insufficient_evidence"],
        },
        model_version="gpt-5.6-sol",
        input_tokens=17,
        output_tokens=9,
    )
    initialize, initialized, thread_start, turn_start = websocket.sent
    assert initialize == {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "clientInfo": {"name": "hotkey-agent", "title": "HotKey Agent", "version": "0.1.0"}
        },
    }
    assert initialized == {"jsonrpc": "2.0", "method": "initialized", "params": {}}
    thread_params = thread_start["params"]
    assert thread_params["ephemeral"] is True
    assert thread_params["cwd"] == "/tmp"
    assert thread_params["approvalPolicy"] == "never"
    assert thread_params["sandbox"] == "read-only"
    assert all(value is False for value in thread_params["config"]["features"].values())
    assert thread_params["config"]["mcp_servers"] == {}
    assert "Synthetic evidence only." not in json.dumps(thread_params)
    turn_params = turn_start["params"]
    assert turn_params["approvalPolicy"] == "never"
    assert turn_params["sandboxPolicy"] == {"type": "readOnly", "networkAccess": False}
    codex_schema = turn_params["outputSchema"]
    assert codex_schema["required"] == ["decision", "score", "reason_codes", "note"]
    assert codex_schema["properties"]["note"] == {"anyOf": [{"type": "string"}, {"type": "null"}]}
    assert "minimum" not in json.dumps(codex_schema)
    assert "uniqueItems" not in json.dumps(codex_schema)
    assert "Synthetic evidence only." in turn_params["input"][0]["text"]
    assert TOKEN not in json.dumps(websocket.sent)


def test_codex_client_keeps_repair_data_in_the_bounded_turn_only() -> None:
    websocket = FakeWebSocket(_success_messages())
    client = CodexAppServerClient(_settings(), connector=_connector(websocket))
    request = _request()
    completion = asyncio.run(
        client.complete(
            schema_name=str(request.payload["schema_name"]),
            instruction=str(request.payload["instruction"]),
            input_value=request.payload["input"],
            output_schema=request.payload["schema"],
            repair={"violations": ["retry"]},
        )
    )
    assert completion.output_tokens == 9
    assert "retry" in websocket.sent[3]["params"]["input"][0]["text"]

    with pytest.raises(ModelOutputInvalidError):
        asyncio.run(
            client.complete(
                schema_name="schema-v1",
                instruction="x" * 1_048_577,
                input_value={},
                output_schema={},
                repair=None,
            )
        )


def test_codex_analyzer_returns_structured_result_with_actual_model_and_usage() -> None:
    class ClientFake:
        async def complete(self, **_arguments: Any) -> ModelCompletion:
            return ModelCompletion(
                value={
                    "decision": "review",
                    "score": 0,
                    "reason_codes": ["insufficient_evidence"],
                },
                model_version="gpt-5.6-sol",
                input_tokens=17,
                output_tokens=9,
            )

    response = asyncio.run(
        CodexAppServerAnalyzer(_settings(), client=ClientFake()).analyze(_request())
    )
    assert response.status == "succeeded"
    assert response.runtime.name == "codex_app_server"
    assert response.runtime.version == "gpt-5.6-sol"
    assert response.runtime.degraded is False
    assert response.usage is not None
    assert response.usage.input_tokens == 17
    assert response.usage.output_tokens == 9
    assert response.suggestions[0].evidence_ids == ["evidence-1"]
    assert response.suggestions[0].confidence == 0


@pytest.mark.parametrize(
    ("messages", "error_type"),
    [
        (
            [{"jsonrpc": "2.0", "id": 1, "error": {"code": -32603, "message": "secret"}}],
            ModelRuntimeError,
        ),
        (
            [
                {"jsonrpc": "2.0", "id": 1, "result": {}},
                {"jsonrpc": "2.0", "id": 99, "method": "item/commandExecution/request"},
            ],
            ModelRuntimeError,
        ),
        (_success_messages(item_type="commandExecution"), ModelOutputInvalidError),
    ],
)
def test_codex_client_rejects_protocol_errors_server_requests_and_tool_items(
    messages: list[object], error_type: type[ModelRuntimeError]
) -> None:
    client = CodexAppServerClient(_settings(), connector=_connector(FakeWebSocket(messages)))
    with pytest.raises(error_type) as captured:
        asyncio.run(_complete(client))
    assert "secret" not in str(captured.value)


def test_codex_client_maps_usage_limits_and_rejects_invalid_turns_and_outputs() -> None:
    limited = _success_messages()
    limited[-1] = {
        "jsonrpc": "2.0",
        "method": "turn/completed",
        "params": {
            "threadId": "thread-1",
            "turn": {
                "id": "turn-1",
                "status": "failed",
                "items": [],
                "error": {"code": "usageLimitExceeded", "message": "private"},
            },
        },
    }
    with pytest.raises(ModelRateLimitedError):
        asyncio.run(
            _complete(
                CodexAppServerClient(_settings(), connector=_connector(FakeWebSocket(limited)))
            )
        )

    invalid_cases = []
    for mutation in ("missing_usage", "malformed_output", "wrong_thread", "failed"):
        messages = _success_messages()
        if mutation == "missing_usage":
            messages.pop(-2)
        elif mutation == "malformed_output":
            messages[-1]["params"]["turn"]["items"][0]["text"] = "not-json"  # type: ignore[index]
        elif mutation == "wrong_thread":
            messages[-1]["params"]["threadId"] = "thread-other"  # type: ignore[index]
        else:
            messages[-1]["params"]["turn"]["status"] = "interrupted"  # type: ignore[index]
        invalid_cases.append(messages)
    for messages in invalid_cases:
        with pytest.raises(ModelOutputInvalidError):
            asyncio.run(
                _complete(
                    CodexAppServerClient(_settings(), connector=_connector(FakeWebSocket(messages)))
                )
            )


def test_codex_client_maps_timeout_connection_malformed_and_oversized_messages() -> None:
    with pytest.raises(ModelTimeoutError):
        asyncio.run(
            _complete(
                CodexAppServerClient(
                    _settings(),
                    connector=_connector(FakeWebSocket([TimeoutError("private timeout")])),
                )
            )
        )
    with pytest.raises(ModelRuntimeError):
        asyncio.run(
            _complete(
                CodexAppServerClient(
                    _settings(), connector=_connector(FakeWebSocket([OSError("private socket")]))
                )
            )
        )
    for value in ("not-json", b"binary", "x" * 1_048_577):
        with pytest.raises(ModelOutputInvalidError):
            asyncio.run(
                _complete(
                    CodexAppServerClient(_settings(), connector=_connector(FakeWebSocket([value])))
                )
            )


def test_codex_client_fails_closed_for_response_and_notification_edge_cases() -> None:
    base = _success_messages()
    for messages in (
        [
            base[0],
            {"method": "configWarning", "params": {"summary": "ignored"}},
            {"method": "mcpServer/startupStatus/updated", "params": {"status": "failed"}},
            {"method": "remoteControl/status/changed", "params": {"status": "disabled"}},
            {"method": "warning", "params": {"message": "ignored"}},
            {"jsonrpc": "2.0", "method": "thread/started", "params": {}},
            *base[1:],
        ],
        [
            *base[:3],
            {
                "jsonrpc": "2.0",
                "method": "item/started",
                "params": {"item": {"type": "reasoning", "id": "reasoning-1"}},
            },
            *base[3:],
        ],
    ):
        assert (
            asyncio.run(
                _complete(
                    CodexAppServerClient(_settings(), connector=_connector(FakeWebSocket(messages)))
                )
            ).output_tokens
            == 9
        )

    cases: list[tuple[list[object], type[ModelRuntimeError]]] = [
        ([{"jsonrpc": "2.0", "id": 7, "result": {}}], ModelOutputInvalidError),
        ([{"jsonrpc": "2.0", "id": 1, "result": None}], ModelOutputInvalidError),
        (
            [
                base[0],
                {"jsonrpc": "2.0", "method": 7, "params": {}},
                *base[1:],
            ],
            ModelOutputInvalidError,
        ),
        (
            [
                *base[:3],
                {"jsonrpc": "2.0", "id": 88, "result": {}},
            ],
            ModelRuntimeError,
        ),
        (
            [
                base[0],
                {"jsonrpc": "2.0", "method": "unknown/notification", "params": {}},
                *base[1:],
            ],
            ModelOutputInvalidError,
        ),
        (
            [
                *base[:3],
                {"jsonrpc": "2.0", "method": "unknown/notification", "params": {}},
            ],
            ModelOutputInvalidError,
        ),
        (
            [
                *base[:3],
                {
                    "jsonrpc": "2.0",
                    "method": "error",
                    "params": {"error": {"code": "usageLimitExceeded"}},
                },
            ],
            ModelRateLimitedError,
        ),
        (
            [
                *base[:3],
                {"jsonrpc": "2.0", "method": "error", "params": {"code": "internal"}},
            ],
            ModelRuntimeError,
        ),
    ]
    for messages, error_type in cases:
        with pytest.raises(error_type):
            asyncio.run(
                _complete(
                    CodexAppServerClient(_settings(), connector=_connector(FakeWebSocket(messages)))
                )
            )


def test_codex_protocol_identity_usage_and_output_helpers_are_strict() -> None:
    for value in ({}, {"thread": {"id": "thread-1"}, "model": "x" * 33}):
        with pytest.raises(ModelOutputInvalidError):
            _thread_identity(value)
    with pytest.raises(ModelOutputInvalidError):
        _turn_identity({"turn": {"id": "bad id"}})
    with pytest.raises(ModelOutputInvalidError):
        _token_usage(
            {
                "threadId": "thread-1",
                "turnId": "turn-1",
                "tokenUsage": {"last": {"inputTokens": True, "outputTokens": 1}},
            },
            thread_id="thread-1",
            turn_id="turn-1",
        )
    with pytest.raises(ModelRuntimeError):
        _completed_value(
            {
                "threadId": "thread-1",
                "turn": {
                    "id": "turn-1",
                    "status": "failed",
                    "items": [],
                    "error": {"code": "internal"},
                },
            },
            thread_id="thread-1",
            turn_id="turn-1",
        )
    for turn in (
        {"id": "turn-1", "status": "completed", "items": None},
        {
            "id": "turn-1",
            "status": "completed",
            "items": [{"type": "agentMessage", "id": "item-1", "text": []}],
        },
        {
            "id": "turn-1",
            "status": "completed",
            "items": [{"type": "reasoning", "id": "item-1"}],
        },
        {
            "id": "turn-1",
            "status": "completed",
            "items": [{"type": "agentMessage", "id": "item-1", "text": "[]"}],
        },
    ):
        with pytest.raises(ModelOutputInvalidError):
            _completed_value(
                {"threadId": "thread-1", "turn": turn},
                thread_id="thread-1",
                turn_id="turn-1",
            )
    assert (
        _completed_value(
            {
                "threadId": "thread-1",
                "turn": {
                    "id": "turn-1",
                    "status": "completed",
                    "items": [{"type": "agentMessage", "id": "item-1", "text": "{}"}],
                },
            },
            thread_id="thread-1",
            turn_id="turn-1",
        )
        == {}
    )
    assert _error_code({"error": {"code": "nested"}}) == "nested"
    assert _error_code("invalid") is None


def test_codex_schema_adapter_keeps_structure_and_defers_full_constraints() -> None:
    canonical = {
        "type": "object",
        "additionalProperties": False,
        "required": ["items"],
        "properties": {
            "items": {
                "type": "array",
                "minItems": 1,
                "items": {
                    "type": "object",
                    "additionalProperties": False,
                    "required": ["value"],
                    "properties": {
                        "value": {"type": "integer", "minimum": 1},
                        "label": {"type": "string", "maxLength": 20},
                    },
                },
            }
        },
    }
    adapted = _codex_output_schema(canonical)
    nested = adapted["properties"]["items"]["items"]
    assert nested["required"] == ["value", "label"]
    assert nested["properties"]["label"] == {"anyOf": [{"type": "string"}, {"type": "null"}]}
    assert "minimum" not in json.dumps(adapted)
    assert _strip_optional_nulls({"items": [{"value": 1, "label": None}]}, canonical) == {
        "items": [{"value": 1}]
    }
    assert _codex_output_schema(
        {"anyOf": [{"type": "string", "enum": ["ok"]}, {"type": "null"}]}
    ) == {"anyOf": [{"type": "string", "enum": ["ok"]}, {"type": "null"}]}

    for invalid in (
        None,
        {},
        {"type": "array"},
        {"type": "object", "properties": []},
        {
            "type": "object",
            "properties": {"value": {"type": "string"}},
            "required": ["missing"],
        },
    ):
        with pytest.raises(ModelOutputInvalidError):
            _codex_output_schema(invalid)
