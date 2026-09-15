from pathlib import Path

import pytest

from analysis.evaluation import (
    MODEL_OUTPUT_ADAPTER,
    CaseObservation,
    EvaluationFixture,
    LabeledDecision,
    expected_prediction,
    loopback_endpoint,
    preflight_decision,
    score_suite,
)

FIXTURE = Path(__file__).parents[1] / "fixtures" / "analysis" / "model-evaluation-v1.json"


def test_fixture_and_perfect_predictions_pass_fixed_thresholds():
    fixture = EvaluationFixture.model_validate_json(FIXTURE.read_text())
    observations = [
        CaseObservation(
            case_id=case.id,
            prediction=expected_prediction(case),
            wall_seconds=1,
            resident_bytes=8 * 1024**3,
        )
        for case in fixture.cases
    ]
    result = score_suite(fixture, observations)

    assert result["passed"] is True
    assert result["metrics"]["structured_rate"] == 1
    assert result["metrics"]["citation_support_rate"] == 1
    assert result["metrics"]["injection_safe_rate"] == 1


def test_unknown_citation_and_failed_abstention_fail_gate():
    fixture = EvaluationFixture.model_validate_json(FIXTURE.read_text())
    observations = [
        CaseObservation(
            case_id=case.id,
            prediction=expected_prediction(case),
            wall_seconds=1,
        )
        for case in fixture.cases
    ]
    injection = next(
        index for index, case in enumerate(fixture.cases) if case.category == "prompt_injection"
    )
    observations[injection] = CaseObservation(
        case_id=fixture.cases[injection].id,
        prediction=LabeledDecision(
            decision="label",
            topic="feature_value",
            target="publishing",
            sentiment="positive",
            stance="support",
            request="",
            citation_ids=["invented"],
        ),
        wall_seconds=1,
    )
    result = score_suite(fixture, observations)

    assert result["passed"] is False
    assert result["metrics"]["citation_whitelist_rate"] < 1
    assert result["metrics"]["injection_safe_rate"] == 0


def test_only_plain_http_loopback_ollama_endpoint_is_allowed():
    assert loopback_endpoint("http://127.0.0.1:11434") == "http://127.0.0.1:11434"
    assert loopback_endpoint("http://[::1]:11434/") == "http://[::1]:11434"
    with pytest.raises(ValueError):
        loopback_endpoint("https://api.example.test:11434")
    with pytest.raises(ValueError):
        loopback_endpoint("http://192.0.2.1:11434")


def test_preflight_abstains_without_one_resolvable_target():
    fixture = EvaluationFixture.model_validate_json(FIXTURE.read_text())
    by_category = {case.category: case for case in fixture.cases}

    assert preflight_decision(by_category["missing_context"].contexts).reason == "missing_context"
    assert preflight_decision(by_category["multi_target"].contexts).reason == "multi_target"
    assert preflight_decision(by_category["parent_context"].contexts) is None


def test_model_output_union_rejects_contradictory_or_extra_fields():
    with pytest.raises(ValueError):
        MODEL_OUTPUT_ADAPTER.validate_python(
            {
                "decision": "label",
                "topic": "unknown",
                "target": "unknown",
                "sentiment": "unknown",
                "stance": "unknown",
                "citation_ids": [],
            }
        )
    with pytest.raises(ValueError):
        MODEL_OUTPUT_ADAPTER.validate_python(
            {
                "decision": "abstain",
                "reason": "missing_context",
                "citation_ids": ["invented"],
            }
        )
