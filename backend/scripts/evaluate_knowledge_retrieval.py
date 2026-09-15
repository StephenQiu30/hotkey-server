import argparse
import json
import math
import platform
import time
from pathlib import Path
from statistics import quantiles

import httpx

from knowledge.retrieval_evaluation import (
    RetrievalFixture,
    lexical_ranking,
    passes_gate,
    score,
    validate_loopback_origin,
    vector_ranking,
)


def percentile_95(values: list[float]) -> float:
    return quantiles(values, n=100, method="inclusive")[94] if len(values) > 1 else values[0]


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--fixture", type=Path, required=True)
    parser.add_argument("--model", default="qwen3-embedding:latest")
    parser.add_argument("--endpoint", default="http://127.0.0.1:11434")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    endpoint = validate_loopback_origin(args.endpoint)
    fixture = RetrievalFixture.model_validate(json.loads(args.fixture.read_text()))
    with httpx.Client(base_url=endpoint, timeout=60, trust_env=False) as client:
        tags = client.get("/api/tags").raise_for_status().json()["models"]
        model = next(item for item in tags if item["name"] == args.model)
        document_response = client.post(
            "/api/embed",
            json={
                "model": args.model,
                "input": [document.text for document in fixture.documents],
                "dimensions": 1024,
                "truncate": False,
                "keep_alive": "5m",
            },
        )
        document_response.raise_for_status()
        document_vectors = dict(
            zip(
                [document.id for document in fixture.documents],
                document_response.json()["embeddings"],
                strict=True,
            )
        )
        query_vectors: list[list[float]] = []
        query_seconds: list[float] = []
        for query in fixture.queries:
            started = time.perf_counter()
            response = client.post(
                "/api/embed",
                json={
                    "model": args.model,
                    "input": query.text,
                    "dimensions": 1024,
                    "truncate": False,
                    "keep_alive": "5m",
                },
            )
            response.raise_for_status()
            query_seconds.append(time.perf_counter() - started)
            query_vectors.append(response.json()["embeddings"][0])
        resident = 0
        for process in client.get("/api/ps").raise_for_status().json().get("models", []):
            if process.get("name") == args.model:
                resident = int(process.get("size_vram") or process.get("size") or 0)
        client.post(
            "/api/embed",
            json={"model": args.model, "input": "unload", "dimensions": 1024, "keep_alive": 0},
        ).raise_for_status()

    vectors = list(document_vectors.values()) + query_vectors
    dimensions = sorted({len(vector) for vector in vectors})
    maximum_norm_error = max(
        abs(math.sqrt(sum(value * value for value in vector)) - 1) for vector in vectors
    )
    lexical = score(
        [lexical_ranking(query.text, fixture.documents) for query in fixture.queries],
        fixture.queries,
    )
    vector = score(
        [vector_ranking(query, document_vectors) for query in query_vectors], fixture.queries
    )
    p95 = percentile_95(query_seconds)
    quality_passed = passes_gate(vector, lexical)
    runtime_passed = (
        dimensions == [1024]
        and maximum_norm_error <= 0.001
        and p95 <= 2
        and resident <= 12 * 1024**3
    )
    report = {
        "fixture": fixture.version,
        "synthetic": fixture.synthetic,
        "model": {
            "name": model["name"],
            "digest": model["digest"],
            "size": model["size"],
        },
        "environment": {
            "system": platform.system(),
            "machine": platform.machine(),
            "endpoint": endpoint,
            "trust_env": False,
        },
        "metrics": {
            "lexical": lexical.__dict__,
            "vector": vector.__dict__,
            "query_p95_seconds": p95,
            "dimensions": dimensions,
            "maximum_norm_error": maximum_norm_error,
            "resident_bytes": resident,
        },
        "passed": quality_passed and runtime_passed,
    }
    payload = json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    if args.output:
        args.output.write_text(payload)
    print(payload, end="")
    if not report["passed"]:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
