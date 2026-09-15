import math

import httpx

from ai.contracts import EmbeddingBatch, EmbeddingProviderError


class OllamaEmbeddingProvider:
    def __init__(
        self,
        *,
        base_url: str,
        model: str,
        model_digest: str,
        dimensions: int,
        timeout_seconds: float,
        transport: httpx.BaseTransport | None = None,
    ):
        self.base_url = base_url
        self.model = model
        self.model_digest = model_digest
        self.dimensions = dimensions
        self.timeout_seconds = timeout_seconds
        self.transport = transport

    def embed(self, texts: list[str]) -> EmbeddingBatch:
        if not texts or any(not text.strip() for text in texts):
            raise EmbeddingProviderError("embedding_input_invalid")
        try:
            with httpx.Client(
                base_url=self.base_url,
                timeout=self.timeout_seconds,
                transport=self.transport,
                trust_env=False,
            ) as client:
                response = client.post(
                    "/api/embed",
                    json={
                        "model": self.model,
                        "input": texts,
                        "dimensions": self.dimensions,
                        "truncate": False,
                        "keep_alive": "5m",
                    },
                )
                response.raise_for_status()
        except httpx.HTTPError as error:
            raise EmbeddingProviderError("embedding_unavailable") from error
        try:
            payload = response.json()
        except (ValueError, TypeError) as error:
            raise EmbeddingProviderError("embedding_invalid_response") from error

        if not isinstance(payload, dict) or payload.get("model") != self.model:
            raise EmbeddingProviderError("embedding_invalid_response")
        vectors = payload.get("embeddings")
        if not isinstance(vectors, list) or len(vectors) != len(texts):
            raise EmbeddingProviderError("embedding_invalid_response")
        validated: list[list[float]] = []
        for value in vectors:
            if not isinstance(value, list) or len(value) != self.dimensions:
                raise EmbeddingProviderError("embedding_invalid_response")
            if any(not isinstance(item, int | float) or not math.isfinite(item) for item in value):
                raise EmbeddingProviderError("embedding_invalid_response")
            vector = [float(item) for item in value]
            norm = math.sqrt(sum(item * item for item in vector))
            if abs(norm - 1.0) > 0.01:
                raise EmbeddingProviderError("embedding_invalid_response")
            validated.append(vector)
        return EmbeddingBatch(
            model=self.model,
            model_digest=self.model_digest,
            dimensions=self.dimensions,
            vectors=validated,
        )
