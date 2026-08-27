from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path


def test_capacity_tool_reports_bounded_authenticated_analysis_case() -> None:
    tool = Path(__file__).parent / "tools" / "capacity_baseline.py"
    completed = subprocess.run(
        [
            sys.executable,
            str(tool),
            "--tasks",
            "relevance",
            "--contexts",
            "small",
            "--concurrencies",
            "2",
            "--requests-per-worker",
            "1",
        ],
        check=True,
        capture_output=True,
        text=True,
    )
    report = json.loads(completed.stdout)

    assert report["schema_version"] == "agent-capacity.v1"
    assert report["environment"]["transport"] == "authenticated-asgi-http"
    assert len(report["cases"]) == 1
    case = report["cases"][0]
    assert case["task"] == "relevance"
    assert case["context"] == "small"
    assert case["concurrency"] == 2
    assert case["requests"] == 2
    assert case["successes"] == 2
    assert case["failures"] == 0
    assert case["failure_categories"] == {}
    assert case["request_bytes"] > case["context_bytes"] > 0
    assert case["wall_ms"] >= 0
    assert case["cpu_ms"] >= 0
    assert case["peak_rss_bytes"] > 0
