import argparse
import json
import platform
import sys
import time
from pathlib import Path
from typing import Any, cast

import httpx

from analysis.evaluation import (
    MODEL_OUTPUT_ADAPTER,
    CaseObservation,
    EvaluationCase,
    EvaluationFixture,
    build_prompt,
    loopback_endpoint,
    preflight_decision,
    score_suite,
)


def _json_object(response: httpx.Response) -> dict[str, Any]:
    response.raise_for_status()
    value = response.json()
    if not isinstance(value, dict):
        raise ValueError("Ollama returned a non-object response")
    return cast(dict[str, Any], value)


def _resident_bytes(client: httpx.Client, model: str) -> int:
    payload = _json_object(client.get("/api/ps"))
    models = payload.get("models", [])
    if not isinstance(models, list):
        return 0
    for item in models:
        if isinstance(item, dict) and item.get("name") == model:
            size = item.get("size_vram", item.get("size", 0))
            return int(size) if isinstance(size, int | float) else 0
    return 0


def _observe_case(client: httpx.Client, model: str, case: EvaluationCase) -> CaseObservation:
    started = time.perf_counter()
    preflight = preflight_decision(case.contexts)
    if preflight is not None:
        return CaseObservation(
            case_id=case.id,
            prediction=preflight,
            wall_seconds=time.perf_counter() - started,
        )
    system, user = build_prompt(case)
    try:
        payload = _json_object(
            client.post(
                "/api/chat",
                json={
                    "model": model,
                    "messages": [
                        {"role": "system", "content": system},
                        {"role": "user", "content": user},
                    ],
                    "format": MODEL_OUTPUT_ADAPTER.json_schema(),
                    "stream": False,
                    "think": False,
                    "keep_alive": "5m",
                    "options": {
                        "temperature": 0,
                        "seed": 42,
                        "num_predict": 256,
                    },
                },
            )
        )
        message = payload.get("message")
        if not isinstance(message, dict) or not isinstance(message.get("content"), str):
            raise ValueError("Ollama response did not contain message.content")
        prediction = MODEL_OUTPUT_ADAPTER.validate_json(message["content"])
        return CaseObservation(
            case_id=case.id,
            prediction=prediction,
            wall_seconds=time.perf_counter() - started,
            total_duration_ns=int(payload.get("total_duration", 0)),
            prompt_tokens=int(payload.get("prompt_eval_count", 0)),
            output_tokens=int(payload.get("eval_count", 0)),
            resident_bytes=_resident_bytes(client, model),
        )
    except Exception as error:
        try:
            resident_bytes = _resident_bytes(client, model)
        except Exception:
            resident_bytes = 0
        return CaseObservation(
            case_id=case.id,
            error=f"{type(error).__name__}: {error}",
            wall_seconds=time.perf_counter() - started,
            resident_bytes=resident_bytes,
        )


def _unload(client: httpx.Client, model: str) -> None:
    _json_object(
        client.post(
            "/api/generate",
            json={"model": model, "prompt": "", "stream": False, "keep_alive": 0},
        )
    )


def _model_metadata(client: httpx.Client, model: str) -> dict[str, object]:
    payload = _json_object(client.get("/api/tags"))
    models = payload.get("models", [])
    if not isinstance(models, list):
        return {"name": model}
    for item in models:
        if isinstance(item, dict) and item.get("name") == model:
            return {
                "name": model,
                "digest": item.get("digest"),
                "size_bytes": item.get("size"),
                "modified_at": item.get("modified_at"),
            }
    return {"name": model}


def evaluate(
    *, endpoint: str, fixture_path: Path, models: list[str], timeout_seconds: float
) -> dict[str, object]:
    fixture = EvaluationFixture.model_validate_json(fixture_path.read_text())
    with httpx.Client(
        base_url=endpoint,
        timeout=timeout_seconds,
        trust_env=False,
    ) as client:
        version = _json_object(client.get("/api/version")).get("version")
        reports: list[dict[str, object]] = []
        for model in models:
            observations: list[CaseObservation] = []
            try:
                observations = [_observe_case(client, model, case) for case in fixture.cases]
                scored = score_suite(fixture, observations)
                scored["model"] = _model_metadata(client, model)
                scored["errors"] = [
                    {"case_id": item.case_id, "error": item.error}
                    for item in observations
                    if item.error is not None
                ]
                scored["observations"] = [item.model_dump() for item in observations]
                reports.append(scored)
            finally:
                _unload(client, model)
        return {
            "scope": "S04-T02B local structured classification model evaluation",
            "fixture_version": fixture.version,
            "synthetic": fixture.synthetic,
            "environment": {
                "endpoint": endpoint,
                "ollama_version": version,
                "machine": platform.machine(),
                "operating_system": platform.system(),
            },
            "models": reports,
            "eligible_models": [
                cast(dict[str, object], item["model"])["name"]
                for item in reports
                if item["passed"] is True
            ],
        }


def main() -> int:
    default_fixture = (
        Path(__file__).resolve().parents[1]
        / "tests"
        / "fixtures"
        / "analysis"
        / "model-evaluation-v1.json"
    )
    parser = argparse.ArgumentParser()
    parser.add_argument("--endpoint", type=loopback_endpoint, default="http://127.0.0.1:11434")
    parser.add_argument("--fixture", type=Path, default=default_fixture)
    parser.add_argument("--model", action="append", required=True)
    parser.add_argument("--timeout-seconds", type=float, default=45.0)
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    report = evaluate(
        endpoint=args.endpoint,
        fixture_path=args.fixture,
        models=args.model,
        timeout_seconds=args.timeout_seconds,
    )
    rendered = json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True)
    if args.output is not None:
        args.output.write_text(rendered + "\n")
    else:
        print(rendered)
    return 0 if report["eligible_models"] else 2


if __name__ == "__main__":
    sys.exit(main())
