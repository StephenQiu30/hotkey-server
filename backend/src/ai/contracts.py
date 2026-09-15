from dataclasses import dataclass
from typing import Protocol


@dataclass(frozen=True, slots=True)
class EmbeddingBatch:
    model: str
    model_digest: str
    dimensions: int
    vectors: list[list[float]]


class EmbeddingProviderError(Exception):
    def __init__(self, code: str):
        self.code = code
        super().__init__(code)


class EmbeddingProvider(Protocol):
    def embed(self, texts: list[str]) -> EmbeddingBatch: ...
