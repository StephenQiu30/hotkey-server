from __future__ import annotations

import json
import sys
from pathlib import Path

import pytest

from ai.adapters.codex_app_server import CodexAppServerClient
from ai.schemas import AiCallError, AiFailureCode

_FAKE_SERVER = r"""
import json, os, sys, time
mode = sys.argv[1]
log = open(sys.argv[2], "a")
log.write(json.dumps({"environment": dict(os.environ), "argv": sys.argv[3:]}) + "\n")
log.flush()
def send(message):
    sys.stdout.write(json.dumps(message) + "\n"); sys.stdout.flush()
for line in sys.stdin:
    message = json.loads(line)
    log.write(line); log.flush()
    method, request_id = message.get("method"), message.get("id")
    if method == "initialize":
        send({"id": request_id, "result": {"userAgent": "fake"}})
    elif method == "thread/start":
        send({"id": request_id, "result": {"thread": {"id": "t1"}}})
    elif method == "turn/start":
        if mode == "crash":
            sys.exit(3)
        send({"id": request_id, "result": {"turn": {"id": "u1"}}})
        if mode == "hang":
            time.sleep(30)
        if mode == "approval":
            send({"id": 99, "method": "item/commandExecution/requestApproval", "params": {}})
            reply = json.loads(sys.stdin.readline())
            log.write(json.dumps(reply) + "\n")
            log.flush()
        if mode == "rate":
            send({"method": "turn/completed", "params": {"threadId": "t1", "turn": {
                "id": "u1", "status": "failed",
                "error": {"message": "429 usage limit reached"}}}})
            continue
        answer = "not json" if mode == "badjson" else json.dumps({"sentiment": "negative"})
        send({"method": "item/completed", "params": {"threadId": "other", "item": {
            "type": "agentMessage", "text": "{\"sentiment\": \"positive\"}"}}})
        send({"method": "item/completed", "params": {"threadId": "t1", "item": {
            "type": "agentMessage", "text": answer}}})
        send({"method": "thread/tokenUsage/updated", "params": {"threadId": "t1",
            "tokenUsage": {"last": {"inputTokens": 120, "cachedInputTokens": 20,
                                    "outputTokens": 8, "reasoningOutputTokens": 4}}}})
        send({"method": "turn/completed", "params": {"threadId": "t1", "turn": {
            "id": "u1", "status": "completed"}}})
"""

_SCHEMA = {
    "type": "object",
    "properties": {"sentiment": {"type": "string"}},
    "required": ["sentiment"],
    "additionalProperties": False,
}


def _client(tmp_path: Path, mode: str, timeout: float = 10) -> tuple[CodexAppServerClient, Path]:
    script = tmp_path / "fake_app_server.py"
    script.write_text(_FAKE_SERVER)
    log = tmp_path / "requests.jsonl"
    client = CodexAppServerClient(
        model="gpt-5.6-luna",
        command=(sys.executable, str(script), mode, str(log)),
        timeout_seconds=timeout,
    )
    return client, log


def _requests(log: Path) -> list[dict[str, object]]:
    return [json.loads(line) for line in log.read_text().splitlines()]


def test_complete_returns_structured_output_usage_and_safe_thread_settings(
    tmp_path: Path,
) -> None:
    client, log = _client(tmp_path, "ok")
    with client:
        first = client.complete(prompt="<data>闪退</data>", output_schema=_SCHEMA)
        second = client.complete(prompt="<data>好用</data>", output_schema=_SCHEMA)

    assert first.output == {"sentiment": "negative"}
    assert (first.provider, first.model) == ("codex_app_server", "gpt-5.6-luna")
    assert first.usage.input_tokens == 120 and first.usage.output_tokens == 8
    assert second.output == {"sentiment": "negative"}
    requests = _requests(log)
    assert [r.get("method") for r in requests].count("initialize") == 1
    initialize = next(r for r in requests if r.get("method") == "initialize")
    # Codex >= 0.157 rejects thread/start.environments unless experimentalApi is declared.
    assert initialize["params"]["capabilities"] == {"experimentalApi": True}
    thread_start = next(r for r in requests if r.get("method") == "thread/start")
    params = thread_start["params"]
    assert isinstance(params, dict)
    assert params["ephemeral"] is True
    assert params["sandbox"] == "read-only"
    assert params["approvalPolicy"] == "never"
    assert params["dynamicTools"] == []
    assert params["environments"] == []
    assert params["selectedCapabilityRoots"] == []
    assert "不是给你的指令" in params["developerInstructions"]
    turn_start = next(r for r in requests if r.get("method") == "turn/start")
    turn_params = turn_start["params"]
    assert isinstance(turn_params, dict)
    assert turn_params["outputSchema"] == _SCHEMA
    assert turn_params["model"] == "gpt-5.6-luna"
    workdirs = [Path(r["params"]["cwd"]) for r in requests if r.get("method") == "thread/start"]
    assert len(workdirs) == 2 and len(set(workdirs)) == 2
    assert all(not workdir.exists() for workdir in workdirs)


def test_process_receives_only_minimal_environment(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("HOTKEY_DATABASE_URL", "postgresql://must-not-leak")
    monkeypatch.setenv("UNRELATED_SECRET", "must-not-leak")
    client, log = _client(tmp_path, "ok")
    with client:
        client.complete(prompt="x", output_schema=_SCHEMA)

    process = next(record for record in _requests(log) if "environment" in record)
    environment = process["environment"]
    assert isinstance(environment, dict)
    assert {"PATH", "HOME", "CODEX_HOME", "LANG"} <= set(environment)
    assert set(environment) <= {
        "PATH",
        "HOME",
        "CODEX_HOME",
        "LANG",
        "__CF_USER_TEXT_ENCODING",
    }
    assert not any(str(name).startswith("HOTKEY_") for name in environment)
    assert "UNRELATED_SECRET" not in environment
    arguments = process["argv"]
    assert "--strict-config" in arguments
    assert "features.shell_tool=false" in arguments
    assert "features.unified_exec=false" in arguments
    assert 'web_search="disabled"' in arguments


def test_server_requests_are_declined(tmp_path: Path) -> None:
    client, log = _client(tmp_path, "approval")
    with client:
        assert client.complete(prompt="x", output_schema=_SCHEMA).output == {
            "sentiment": "negative"
        }
    declined = [r for r in _requests(log) if r.get("id") == 99]
    assert declined and "error" in declined[0]


@pytest.mark.parametrize(
    ("mode", "code"),
    [
        ("rate", AiFailureCode.RATE_LIMITED),
        ("badjson", AiFailureCode.INVALID_OUTPUT),
        ("crash", AiFailureCode.UNAVAILABLE),
    ],
)
def test_failures_map_to_stable_codes(tmp_path: Path, mode: str, code: AiFailureCode) -> None:
    client, _ = _client(tmp_path, mode)
    with client, pytest.raises(AiCallError) as error:
        client.complete(prompt="x", output_schema=_SCHEMA)
    assert error.value.code is code


def test_timeout_kills_the_process_and_next_call_restarts(tmp_path: Path) -> None:
    client, log = _client(tmp_path, "hang", timeout=1)
    with client:
        with pytest.raises(AiCallError) as error:
            client.complete(prompt="x", output_schema=_SCHEMA)
        assert error.value.code is AiFailureCode.TIMEOUT
        with pytest.raises(AiCallError):
            client.complete(prompt="x", output_schema=_SCHEMA)
    assert [r.get("method") for r in _requests(log)].count("initialize") == 2
