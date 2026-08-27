from __future__ import annotations

from typing import Annotated, Any, Literal

from pydantic import BaseModel, ConfigDict, Field, StringConstraints, model_validator

ContractVersion = Literal["analysis.v1"]
TaskType = Literal[
    "monitor_compile",
    "relevance",
    "event_cluster",
    "claim_evidence",
    "event_summary",
]
Hash = Annotated[str, StringConstraints(pattern=r"^[a-f0-9]{64}$")]
Identifier = Annotated[
    str, StringConstraints(min_length=1, max_length=128, pattern=r"^[A-Za-z0-9_.:-]+$")
]


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class Evidence(StrictModel):
    id: Identifier
    title: str = Field(min_length=1, max_length=500)
    text: str = Field(min_length=1, max_length=20_000)


class AnalyzeRequest(StrictModel):
    contract_version: ContractVersion
    task_id: Identifier
    task_type: TaskType
    input_hash: Hash
    evidence_set_hash: Hash
    payload: dict[str, Any] = Field(default_factory=dict)
    evidence: list[Evidence] = Field(default_factory=list, max_length=32)

    @model_validator(mode="after")
    def evidence_ids_are_unique(self) -> AnalyzeRequest:
        evidence_ids = [item.id for item in self.evidence]
        if len(evidence_ids) != len(set(evidence_ids)):
            raise ValueError("evidence IDs must be unique")
        return self


class Suggestion(StrictModel):
    kind: str = Field(min_length=1, max_length=64)
    value: dict[str, Any]
    confidence: float = Field(ge=0, le=1)
    evidence_ids: list[Identifier] = Field(max_length=32)
    reason: str = Field(min_length=1, max_length=1_000)


class RuntimeInfo(StrictModel):
    name: Literal["deterministic"]
    version: str = Field(min_length=1, max_length=32)
    degraded: bool


class AnalyzeResponse(StrictModel):
    contract_version: ContractVersion
    task_id: Identifier
    task_type: TaskType
    status: Literal["succeeded", "degraded"]
    suggestions: list[Suggestion] = Field(max_length=32)
    runtime: RuntimeInfo


class HealthResponse(StrictModel):
    status: Literal["ok", "not_ready"]
    service: Literal["hotkey-agent"] = "hotkey-agent"
    version: str


class ErrorBody(StrictModel):
    code: str
    message: str


class ErrorResponse(StrictModel):
    error: ErrorBody
