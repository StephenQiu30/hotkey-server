import math
import unicodedata
from dataclasses import dataclass
from typing import Self
from urllib.parse import urlsplit

from pydantic import BaseModel, ConfigDict, Field, model_validator


class EvaluationDocument(BaseModel):
    model_config = ConfigDict(extra="forbid")
    id: str = Field(pattern=r"^[a-z][a-z0-9_]+$")
    event: str = Field(min_length=1)
    text: str = Field(min_length=10)


class EvaluationQuery(BaseModel):
    model_config = ConfigDict(extra="forbid")
    id: str = Field(pattern=r"^q[0-9]{2}$")
    text: str = Field(min_length=4)
    expected: str


class RetrievalFixture(BaseModel):
    model_config = ConfigDict(extra="forbid")
    version: str
    synthetic: bool
    documents: list[EvaluationDocument] = Field(min_length=8)
    queries: list[EvaluationQuery] = Field(min_length=16)

    @model_validator(mode="after")
    def validate_references(self) -> Self:
        document_ids = [document.id for document in self.documents]
        if len(document_ids) != len(set(document_ids)):
            raise ValueError("document ids must be unique")
        query_ids = [query.id for query in self.queries]
        if len(query_ids) != len(set(query_ids)):
            raise ValueError("query ids must be unique")
        if not set(query.expected for query in self.queries) <= set(document_ids):
            raise ValueError("query expected ids must reference documents")
        if not self.synthetic:
            raise ValueError("evaluation fixture must be explicitly synthetic")
        return self


@dataclass(frozen=True)
class RetrievalMetrics:
    top1: float
    recall_at_3: float
    mrr: float


def normalize(value: str) -> str:
    return " ".join(unicodedata.normalize("NFKC", value).casefold().split())


def validate_loopback_origin(value: str) -> str:
    parsed = urlsplit(value)
    if (
        parsed.scheme != "http"
        or parsed.hostname not in {"127.0.0.1", "localhost", "::1"}
        or parsed.username
        or parsed.password
        or parsed.path.rstrip("/")
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError("embedding endpoint must be a plain HTTP loopback origin")
    return value.rstrip("/")


def lexical_ranking(query: str, documents: list[EvaluationDocument]) -> list[str]:
    needle = normalize(query)
    return [document.id for document in documents if needle in normalize(document.text)]


def cosine(left: list[float], right: list[float]) -> float:
    if len(left) != len(right) or not left:
        raise ValueError("vectors must have the same nonzero dimension")
    left_norm = math.sqrt(sum(value * value for value in left))
    right_norm = math.sqrt(sum(value * value for value in right))
    if left_norm == 0 or right_norm == 0:
        raise ValueError("zero vectors are not searchable")
    return sum(a * b for a, b in zip(left, right, strict=True)) / (left_norm * right_norm)


def vector_ranking(query: list[float], document_vectors: dict[str, list[float]]) -> list[str]:
    return [
        identity
        for identity, _ in sorted(
            document_vectors.items(),
            key=lambda item: (-cosine(query, item[1]), item[0]),
        )
    ]


def score(rankings: list[list[str]], queries: list[EvaluationQuery]) -> RetrievalMetrics:
    if len(rankings) != len(queries):
        raise ValueError("one ranking is required for each query")
    top1 = 0
    recalled = 0
    reciprocal_sum = 0.0
    for ranking, query in zip(rankings, queries, strict=True):
        if ranking and ranking[0] == query.expected:
            top1 += 1
        if query.expected in ranking[:3]:
            recalled += 1
        if query.expected in ranking:
            reciprocal_sum += 1 / (ranking.index(query.expected) + 1)
    total = len(queries)
    return RetrievalMetrics(top1 / total, recalled / total, reciprocal_sum / total)


def passes_gate(vector: RetrievalMetrics, lexical: RetrievalMetrics) -> bool:
    return (
        vector.top1 >= 0.75
        and vector.recall_at_3 >= 0.90
        and vector.mrr >= 0.80
        and vector.recall_at_3 - lexical.recall_at_3 >= 0.40
    )
