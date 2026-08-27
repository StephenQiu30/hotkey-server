from __future__ import annotations

import asyncio

import pytest

from hotkey_agent.analyzer import DeterministicAnalyzer
from hotkey_agent.contracts import AnalyzeRequest, Evidence

HASH = "b" * 64


def _request(task_type: str, *, query: str = "topic") -> AnalyzeRequest:
    return AnalyzeRequest(
        contract_version="analysis.v1",
        task_id="task-1",
        task_type=task_type,  # type: ignore[arg-type]
        input_hash=HASH,
        evidence_set_hash=HASH,
        payload={"query": query},
        evidence=[Evidence(id="ev-1", title="First title", text="Topic evidence")],
    )


@pytest.mark.parametrize(
    ("task_type", "expected_key"),
    [
        ("monitor_compile", "keywords"),
        ("event_cluster", "candidate_key"),
        ("claim_evidence", "candidate_evidence_ids"),
        ("event_summary", "candidate_title"),
    ],
)
def test_deterministic_fallback_marks_non_model_outputs_degraded(
    task_type: str,
    expected_key: str,
) -> None:
    response = asyncio.run(DeterministicAnalyzer().analyze(_request(task_type)))
    assert response.status == "degraded"
    assert response.runtime.degraded is True
    assert expected_key in response.suggestions[0].value
    assert response.suggestions[0].evidence_ids == ["ev-1"]


def test_empty_query_and_evidence_remain_reviewable_without_inventing_confidence() -> None:
    request = _request("relevance", query="")
    request.evidence = []
    response = asyncio.run(DeterministicAnalyzer().analyze(request))
    suggestion = response.suggestions[0]
    assert suggestion.confidence == 0
    assert suggestion.value == {"score": 0.0, "decision": "review"}
    assert suggestion.evidence_ids == []
