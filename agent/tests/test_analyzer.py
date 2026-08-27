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


@pytest.mark.parametrize(
    ("schema_name", "expected_value"),
    [
        ("term-expansion-output-v1", {"terms": []}),
        (
            "relevance-review-output-v1",
            {"decision": "review", "score": 0.0, "reason_codes": ["insufficient_evidence"]},
        ),
        (
            "event-cluster-output-v1",
            {"action": "create", "confidence": 0.0, "reason_codes": ["no_candidate"]},
        ),
        ("event-summary-output-v1", {"title_zh": "待分析事件", "sentences": []}),
        ("entity-claim-output-v1", {"entities": [], "claims": []}),
    ],
)
def test_structured_schema_fallbacks_are_bounded_and_non_authoritative(
    schema_name: str, expected_value: dict[str, object]
) -> None:
    request = _request("relevance")
    request.payload = {"schema_name": schema_name, "input": {}}
    suggestion = asyncio.run(DeterministicAnalyzer().analyze(request)).suggestions[0]
    assert suggestion.value == expected_value
    assert suggestion.confidence == 0


def test_atomic_claim_fallback_quotes_only_bounded_input_data() -> None:
    request = _request("claim_evidence")
    body = "trusted source body " + ("x" * 5000)
    request.payload = {"schema_name": "atomic-claim-evidence-output-v2", "input": {"body": body}}
    suggestion = asyncio.run(DeterministicAnalyzer().analyze(request)).suggestions[0]
    claim = suggestion.value["claims"][0]
    assert claim["exact_quote"] == body[:4096]
    assert len(claim["exact_quote"]) == 4096
    assert suggestion.confidence == 0


def test_unknown_and_incomplete_schema_requests_remain_explicitly_unsupported() -> None:
    for schema_name, source in [
        ("unknown-output-v1", {}),
        ("atomic-claim-evidence-output-v2", {"body": ""}),
    ]:
        request = _request("claim_evidence")
        request.payload = {"schema_name": schema_name, "input": source}
        suggestion = asyncio.run(DeterministicAnalyzer().analyze(request)).suggestions[0]
        assert suggestion.value in ({"status": "unsupported"}, {"claims": []})
        assert suggestion.confidence == 0


def test_event_summary_fallback_handles_blank_and_missing_titles() -> None:
    request = _request("event_summary")
    request.evidence = [Evidence(id="ev-blank", title=" ", text="source")]
    suggestion = asyncio.run(DeterministicAnalyzer().analyze(request)).suggestions[0]
    assert suggestion.value == {"candidate_title": "Pending analysis"}

    request.evidence = []
    suggestion = asyncio.run(DeterministicAnalyzer().analyze(request)).suggestions[0]
    assert suggestion.value == {"candidate_title": "Pending analysis"}
