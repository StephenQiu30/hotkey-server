from __future__ import annotations

import argparse
import asyncio
import hashlib
import json
import math
import platform
import resource
import subprocess
import sys
import time
from collections import Counter
from pathlib import Path
from typing import Any, cast

import httpx2 as httpx

from hotkey_agent.config import Settings
from hotkey_agent.main import create_app

TOKEN = "capacity-agent-token-0123456789abcdef0123456789abcdef"
CONCURRENCY_CANDIDATES = (2, 3, 4)
TASKS = ("relevance", "claim_evidence")
CONTEXTS = ("small", "medium", "large")
CONTEXT_BYTES = {
    "relevance": {"small": 128, "medium": 600, "large": 1_200},
    "claim_evidence": {"small": 1_024, "medium": 20_000, "large": 80_000},
}
REPOSITORY = Path(__file__).resolve().parents[3]
SCHEMA_ROOT = REPOSITORY / "backend/internal/modules/intelligence/schemas"


def main() -> None:
    parser = argparse.ArgumentParser(description="Measure bounded Python Agent analysis capacity")
    parser.add_argument("--tasks", nargs="+", choices=TASKS, default=list(TASKS))
    parser.add_argument("--contexts", nargs="+", choices=CONTEXTS, default=list(CONTEXTS))
    parser.add_argument(
        "--concurrencies",
        nargs="+",
        type=int,
        choices=CONCURRENCY_CANDIDATES,
        default=list(CONCURRENCY_CANDIDATES),
    )
    parser.add_argument("--requests-per-worker", type=int, default=5)
    parser.add_argument("--child", action="store_true", help=argparse.SUPPRESS)
    arguments = parser.parse_args()
    if arguments.requests_per_worker < 1 or arguments.requests_per_worker > 100:
        parser.error("requests-per-worker must be between 1 and 100")

    if arguments.child:
        if (
            len(arguments.tasks) != 1
            or len(arguments.contexts) != 1
            or len(arguments.concurrencies) != 1
        ):
            parser.error("child mode accepts exactly one task, context and concurrency")
        result = asyncio.run(
            _measure_case(
                arguments.tasks[0],
                arguments.contexts[0],
                arguments.concurrencies[0],
                arguments.requests_per_worker,
            )
        )
        print(json.dumps(result, ensure_ascii=False, sort_keys=True, separators=(",", ":")))
        return

    cases = [
        _run_isolated_case(task, context, concurrency, arguments.requests_per_worker)
        for task in arguments.tasks
        for context in arguments.contexts
        for concurrency in arguments.concurrencies
    ]
    report = {
        "schema_version": "agent-capacity.v1",
        "environment": {
            "machine": platform.machine(),
            "platform": platform.platform(),
            "python": platform.python_version(),
            "transport": "authenticated-asgi-http",
            "requests_per_worker": arguments.requests_per_worker,
        },
        "cases": cases,
    }
    print(json.dumps(report, ensure_ascii=False, sort_keys=True, separators=(",", ":")))


def _run_isolated_case(
    task: str, context: str, concurrency: int, requests_per_worker: int
) -> dict[str, Any]:
    command = [
        sys.executable,
        str(Path(__file__).resolve()),
        "--child",
        "--tasks",
        task,
        "--contexts",
        context,
        "--concurrencies",
        str(concurrency),
        "--requests-per-worker",
        str(requests_per_worker),
    ]
    completed = subprocess.run(
        command,
        check=True,
        capture_output=True,
        text=True,
        timeout=120,
    )
    return cast(dict[str, Any], json.loads(completed.stdout))


async def _measure_case(
    task: str, context: str, concurrency: int, requests_per_worker: int
) -> dict[str, Any]:
    context_bytes = CONTEXT_BYTES[task][context]
    request_template = _request(task, context_bytes, 0)
    request_bytes = len(
        json.dumps(request_template, ensure_ascii=False, separators=(",", ":")).encode()
    )
    application = create_app(
        Settings(
            auth_token=TOKEN,
            runtime="deterministic",
            max_request_bytes=262_144,
            max_concurrency=concurrency,
        )
    )
    transport = httpx.ASGITransport(app=application)
    latencies_ms: list[float] = []
    failure_categories: Counter[str] = Counter()
    request_count = concurrency * requests_per_worker

    async with httpx.AsyncClient(transport=transport, base_url="http://agent.internal") as client:
        cpu_before = resource.getrusage(resource.RUSAGE_SELF)
        wall_started = time.perf_counter_ns()

        async def submit(index: int) -> None:
            started = time.perf_counter_ns()
            try:
                response = await client.post(
                    "/v1/analyze",
                    json=_request(task, context_bytes, index),
                    headers={"X-HotKey-Agent-Token": TOKEN},
                )
                if response.status_code != 200:
                    failure_categories[f"http_{response.status_code}"] += 1
            except Exception as error:
                failure_categories[f"exception_{type(error).__name__}"] += 1
            finally:
                latencies_ms.append((time.perf_counter_ns() - started) / 1_000_000)

        await asyncio.gather(*(submit(index) for index in range(request_count)))
        wall_ms = (time.perf_counter_ns() - wall_started) / 1_000_000
        cpu_after = resource.getrusage(resource.RUSAGE_SELF)

    cpu_ms = (
        cpu_after.ru_utime + cpu_after.ru_stime - cpu_before.ru_utime - cpu_before.ru_stime
    ) * 1_000
    failures = sum(failure_categories.values())
    return {
        "task": task,
        "context": context,
        "context_bytes": context_bytes,
        "request_bytes": request_bytes,
        "concurrency": concurrency,
        "requests": request_count,
        "successes": request_count - failures,
        "failures": failures,
        "failure_categories": dict(sorted(failure_categories.items())),
        "wall_ms": round(wall_ms, 3),
        "cpu_ms": round(cpu_ms, 3),
        "cpu_utilization_percent": round(cpu_ms / wall_ms * 100, 2) if wall_ms else 0.0,
        "latency_p50_ms": round(_percentile(latencies_ms, 0.50), 3),
        "latency_p95_ms": round(_percentile(latencies_ms, 0.95), 3),
        "throughput_per_second": round(request_count * 1_000 / wall_ms, 2) if wall_ms else 0.0,
        "peak_rss_bytes": _peak_rss_bytes(),
    }


def _request(task: str, context_bytes: int, index: int) -> dict[str, Any]:
    content = "x" * context_bytes
    digest = hashlib.sha256(content.encode()).hexdigest()
    if task == "relevance":
        schema_directory = "v1"
        schema_stem = "relevance-review"
        schema_name = "relevance-review-output-v1"
        schema_version = "v1"
        structured_input: dict[str, Any] = {
            "content_excerpt": content,
            "content_language": "en",
            "monitor_intent": "Measure bounded Python analysis",
            "scoring_version": "relevance-v1",
            "scores": {"semantic": 50, "lexical": 50, "entity": 0, "title": 50, "preference": 0},
            "recall_paths": ["lexical"],
            "reason_codes": ["lexical_candidate"],
            "evidence_terms": ["analysis"],
        }
    else:
        schema_directory = "v2"
        schema_stem = "atomic-claim-evidence"
        schema_name = "atomic-claim-evidence-output-v2"
        schema_version = "v2"
        structured_input = {
            "event_id": 1,
            "event_version": 1,
            "event_key": "capacity-event",
            "document_version_id": 1,
            "plaintext_sha256": digest,
            "body": content,
            "body_truncated": False,
        }
    schema_root = SCHEMA_ROOT / schema_directory
    return {
        "contract_version": "analysis.v1",
        "task_id": f"capacity-{task}-{context_bytes}-{index}",
        "task_type": task,
        "input_hash": digest,
        "evidence_set_hash": digest,
        "payload": {
            "schema_name": schema_name,
            "schema_version": schema_version,
            "instruction": "Return only the bounded contract output.",
            "input_schema": json.loads(
                (schema_root / f"{schema_stem}-input.schema.json").read_text()
            ),
            "schema": json.loads((schema_root / f"{schema_stem}-output.schema.json").read_text()),
            "input": structured_input,
        },
        "evidence": [{"id": "evidence-1", "title": "Capacity fixture", "text": content[:20_000]}],
    }


def _percentile(values: list[float], quantile: float) -> float:
    if not values:
        return 0.0
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * quantile) - 1)]


def _peak_rss_bytes() -> int:
    value = int(resource.getrusage(resource.RUSAGE_SELF).ru_maxrss)
    return value if sys.platform == "darwin" else value * 1_024


if __name__ == "__main__":
    main()
