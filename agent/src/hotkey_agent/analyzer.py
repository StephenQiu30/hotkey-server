from __future__ import annotations

import hashlib
import re
from collections.abc import Iterable
from typing import Any

from hotkey_agent.contracts import AnalyzeRequest, AnalyzeResponse, RuntimeInfo, Suggestion

TOKEN_PATTERN = re.compile(r"[\w\u3400-\u9fff]+", re.UNICODE)


class DeterministicAnalyzer:
    """Safe baseline used to prove contracts and fallback behavior, not model quality."""

    version = "deterministic.v1"

    async def analyze(self, request: AnalyzeRequest) -> AnalyzeResponse:
        suggestion = self._suggest(request)
        return AnalyzeResponse(
            contract_version=request.contract_version,
            task_id=request.task_id,
            task_type=request.task_type,
            status="degraded",
            suggestions=[suggestion],
            runtime=RuntimeInfo(name="deterministic", version=self.version, degraded=True),
        )

    def _suggest(self, request: AnalyzeRequest) -> Suggestion:
        evidence_ids = [item.id for item in request.evidence]
        schema_name = str(request.payload.get("schema_name", ""))
        if schema_name:
            schema_value, reason = _schema_fallback(schema_name, request.payload)
            return Suggestion(
                kind=request.task_type,
                value=schema_value,
                confidence=0.0,
                evidence_ids=evidence_ids,
                reason=reason,
            )
        value: dict[str, Any]
        if request.task_type == "relevance":
            query = str(request.payload.get("query", ""))
            score = _overlap_score(
                query, (item.title + " " + item.text for item in request.evidence)
            )
            value = {"score": score, "decision": "relevant" if score >= 0.2 else "review"}
            reason = "Deterministic token overlap; model review remains pending."
        elif request.task_type == "monitor_compile":
            query = str(request.payload.get("query", ""))
            value = {"keywords": sorted(_tokens(query))[:32]}
            reason = "Deterministic keyword extraction; model expansion remains pending."
        elif request.task_type == "event_cluster":
            roots = sorted(_fingerprint(item.text) for item in request.evidence)
            value = {"candidate_key": _fingerprint("|".join(roots)) if roots else ""}
            reason = "Deterministic content fingerprint; semantic clustering remains pending."
        elif request.task_type == "claim_evidence":
            value = {"candidate_evidence_ids": evidence_ids}
            reason = "Evidence whitelist projection only; claim support review remains pending."
        else:
            value = {"candidate_title": _candidate_title(item.title for item in request.evidence)}
            reason = "Evidence-title fallback; generated narrative remains pending."
        return Suggestion(
            kind=request.task_type,
            value=value,
            confidence=0.0,
            evidence_ids=evidence_ids,
            reason=reason,
        )


def _schema_fallback(schema_name: str, payload: dict[str, Any]) -> tuple[dict[str, Any], str]:
    """Return schema-shaped, non-authoritative output for contract testing."""

    if schema_name == "term-expansion-output-v1":
        return {"terms": []}, "Deterministic empty expansion; model expansion remains pending."
    if schema_name == "relevance-review-output-v1":
        return {
            "decision": "review",
            "score": 0.0,
            "reason_codes": ["insufficient_evidence"],
        }, "Deterministic review fallback; model relevance remains pending."
    if schema_name == "event-cluster-output-v1":
        return {
            "action": "create",
            "confidence": 0.0,
            "reason_codes": ["no_candidate"],
        }, "Deterministic new-event fallback; semantic clustering remains pending."
    if schema_name == "event-summary-output-v1":
        return {
            "title_zh": "待分析事件",
            "sentences": [],
        }, "Deterministic title fallback; event narrative remains pending."
    if schema_name == "entity-claim-output-v1":
        return {
            "entities": [],
            "claims": [],
        }, "Deterministic empty extraction; claim analysis remains pending."
    if schema_name == "atomic-claim-evidence-output-v2":
        source = payload.get("input")
        body = source.get("body") if isinstance(source, dict) else None
        if not isinstance(body, str) or not body:
            return {"claims": []}, "Atomic claim input is unavailable; analysis remains pending."
        quote = body[:4096]
        return {
            "claims": [
                {
                    "subject": "pending",
                    "predicate": "pending",
                    "object": "pending",
                    "relation": "unknown",
                    "exact_quote": quote,
                    "relation_score": 0.0,
                    "qualifiers": [],
                }
            ]
        }, "Deterministic exact-quote placeholder; claim analysis remains pending."
    return {"status": "unsupported"}, "Unsupported schema; model analysis remains pending."


def _tokens(value: str) -> set[str]:
    return {
        match.group(0).casefold()
        for match in TOKEN_PATTERN.finditer(value)
        if len(match.group(0)) > 1
    }


def _overlap_score(query: str, documents: Iterable[str]) -> float:
    query_tokens = _tokens(query)
    if not query_tokens:
        return 0.0
    document_tokens = _tokens(" ".join(documents))
    return round(len(query_tokens & document_tokens) / len(query_tokens), 4)


def _fingerprint(value: str) -> str:
    normalized = " ".join(value.casefold().split())
    return hashlib.sha256(normalized.encode()).hexdigest()


def _candidate_title(titles: Iterable[str]) -> str:
    for title in titles:
        normalized = " ".join(title.split())
        if normalized:
            return normalized[:200]
    return "Pending analysis"
