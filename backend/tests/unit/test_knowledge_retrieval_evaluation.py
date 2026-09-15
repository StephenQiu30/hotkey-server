import json
from pathlib import Path

import pytest

from knowledge.retrieval_evaluation import (
    RetrievalFixture,
    RetrievalMetrics,
    cosine,
    lexical_ranking,
    passes_gate,
    score,
    validate_loopback_origin,
    vector_ranking,
)

FIXTURE = Path(__file__).parents[1] / "fixtures/knowledge/retrieval-evaluation-v1.json"


def load_fixture() -> RetrievalFixture:
    return RetrievalFixture.model_validate(json.loads(FIXTURE.read_text()))


def test_fixture_is_synthetic_unique_and_bilingual():
    fixture = load_fixture()
    assert fixture.synthetic and len(fixture.documents) == 8 and len(fixture.queries) == 16
    assert any(query.text.isascii() for query in fixture.queries)
    assert any(not query.text.isascii() for query in fixture.queries)


def test_lexical_baseline_and_gate_are_frozen():
    fixture = load_fixture()
    rankings = [lexical_ranking(query.text, fixture.documents) for query in fixture.queries]
    baseline = score(rankings, fixture.queries)
    assert baseline == RetrievalMetrics(top1=0, recall_at_3=0, mrr=0)
    assert passes_gate(RetrievalMetrics(0.75, 0.9375, 0.82), baseline)
    assert not passes_gate(RetrievalMetrics(0.75, 0.875, 0.82), baseline)


def test_cosine_ranking_is_deterministic_and_rejects_invalid_vectors():
    assert vector_ranking([1.0, 0.0], {"b": [0.0, 1.0], "a": [1.0, 0.0]}) == [
        "a",
        "b",
    ]
    with pytest.raises(ValueError, match="same nonzero dimension"):
        cosine([1.0], [1.0, 0.0])
    with pytest.raises(ValueError, match="zero vectors"):
        cosine([0.0, 0.0], [1.0, 0.0])


def test_embedding_endpoint_is_loopback_only():
    assert validate_loopback_origin("http://127.0.0.1:11434") == "http://127.0.0.1:11434"
    for endpoint in ("https://127.0.0.1:11434", "http://example.com", "http://u:p@localhost"):
        with pytest.raises(ValueError, match="loopback"):
            validate_loopback_origin(endpoint)
