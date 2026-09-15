import json

import httpx
import pytest

from ai.adapters.ollama import OllamaEmbeddingProvider
from ai.contracts import EmbeddingProviderError

DIGEST = "64b933495768fbd3b87c20583d379728a07471e0c66733a9df87cd1901b3c44b"


def _unit_vector() -> list[float]:
    return [1.0, *([0.0] * 1023)]


def test_ollama_embedding_uses_native_batch_protocol_without_environment_proxy():
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/api/embed"
        assert request.headers["content-type"] == "application/json"
        payload = json.loads(request.read())
        assert payload == {
            "model": "qwen3-embedding:latest",
            "input": ["第一条", "second"],
            "dimensions": 1024,
            "truncate": False,
            "keep_alive": "5m",
        }
        return httpx.Response(
            200,
            json={
                "model": "qwen3-embedding:latest",
                "embeddings": [_unit_vector(), _unit_vector()],
            },
        )

    provider = OllamaEmbeddingProvider(
        base_url="http://ollama.internal:11434",
        model="qwen3-embedding:latest",
        model_digest=DIGEST,
        dimensions=1024,
        timeout_seconds=2,
        transport=httpx.MockTransport(handler),
    )
    batch = provider.embed(["第一条", "second"])
    assert batch.model == "qwen3-embedding:latest"
    assert batch.model_digest == DIGEST
    assert batch.dimensions == 1024
    assert len(batch.vectors) == 2


def test_ollama_embedding_rejects_wrong_dimensions_and_non_finite_values():
    non_finite = (
        '{"model":"qwen3-embedding:latest","embeddings":[['
        + ",".join(["1e400", *(["0"] * 1023)])
        + "]] }"
    )
    responses = iter(
        [
            httpx.Response(
                200,
                json={"model": "qwen3-embedding:latest", "embeddings": [[1.0, 0.0]]},
            ),
            httpx.Response(200, content=non_finite, headers={"content-type": "application/json"}),
        ]
    )
    provider = OllamaEmbeddingProvider(
        base_url="http://ollama.internal:11434",
        model="qwen3-embedding:latest",
        model_digest=DIGEST,
        dimensions=1024,
        timeout_seconds=2,
        transport=httpx.MockTransport(lambda _: next(responses)),
    )
    for _ in range(2):
        with pytest.raises(EmbeddingProviderError) as invalid:
            provider.embed(["text"])
        assert invalid.value.code == "embedding_invalid_response"


def test_ollama_embedding_maps_transport_failures_to_stable_error():
    def timeout(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadTimeout("must not leak input", request=request)

    provider = OllamaEmbeddingProvider(
        base_url="http://ollama.internal:11434",
        model="qwen3-embedding:latest",
        model_digest=DIGEST,
        dimensions=1024,
        timeout_seconds=2,
        transport=httpx.MockTransport(timeout),
    )
    with pytest.raises(EmbeddingProviderError) as unavailable:
        provider.embed(["sensitive text"])
    assert unavailable.value.code == "embedding_unavailable"
    assert "sensitive text" not in str(unavailable.value)
