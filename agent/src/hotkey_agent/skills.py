from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from typing import Any

from jsonschema import Draft202012Validator, FormatChecker
from jsonschema.exceptions import SchemaError
from jsonschema.exceptions import ValidationError as JSONSchemaValidationError
from pydantic import Field, ValidationError

from hotkey_agent.contracts import AnalyzeRequest, AnalyzeResponse, StrictModel, TaskType


class SkillContractError(Exception):
    """The caller did not select an exact, trusted analysis skill contract."""


class SkillOutputError(Exception):
    """The analysis engine returned data outside the selected skill contract."""


class StructuredPayload(StrictModel):
    schema_name: str = Field(min_length=1, max_length=128, pattern=r"^[a-z0-9-]+$")
    schema_version: str = Field(min_length=1, max_length=16, pattern=r"^v[1-9][0-9]*$")
    instruction: str = Field(min_length=1, max_length=20_000)
    input_schema: dict[str, Any]
    schema_definition: dict[str, Any] = Field(alias="schema")
    input: dict[str, Any]
    repair: dict[str, Any] | None = None


@dataclass(frozen=True)
class SkillContract:
    task_type: TaskType
    schema_name: str
    schema_version: str
    input_schema_sha256: str
    schema_sha256: str


_CONTRACTS = {
    contract.schema_name: contract
    for contract in (
        SkillContract(
            "monitor_compile",
            "term-expansion-output-v1",
            "v1",
            "b3999c5dd00af52e76b945f60f7af7d83e9e9d133d21e2a4e3251c15789b3ba1",
            "4c724c36d1ddc17cfb121737e3a34acf87034a9778cd03113f5aa983a365749e",
        ),
        SkillContract(
            "relevance",
            "relevance-review-output-v1",
            "v1",
            "20cb7e8685fb373d4cf55b3646aa8d4aa6059ac6f6a8eeaeb03657e9fd22e551",
            "d35728790f7a0b58d0ef5ff5d5022cc19827fe47f3d270668c70894c16497064",
        ),
        SkillContract(
            "event_cluster",
            "event-cluster-output-v1",
            "v1",
            "c6c8274cc6e12f9d33b2dfb684e1559d77665edb8138738f3e5e417417b6467a",
            "cfac0bf23abac1ee5d991a50777f7ef92e2108dfbeb3031a692e815feb17ed45",
        ),
        SkillContract(
            "event_summary",
            "event-summary-output-v1",
            "v1",
            "c166860b8326d6afdeec13383984b938224b490ccb8804b7018592356cd71fe1",
            "7fa7b3d3999d9eea09398404cfd1636af75c54ad1e89595190d5a2417e96c6c2",
        ),
        SkillContract(
            "claim_evidence",
            "entity-claim-output-v1",
            "v1",
            "05dcbf1b020354d9fe9aae160f530e84a9193fc41ec6ca10f07deea4e0901bad",
            "99e37dd647a0cfec325f9fb5aa567e3c18113aea47d3831400ba9543ccd08a08",
        ),
        SkillContract(
            "claim_evidence",
            "atomic-claim-evidence-output-v2",
            "v2",
            "c8ad1e6c7655a89dfe58d59dc3152455af11f799123332d4fdcd100836e6d4c5",
            "0de49b481e69254e744c02ec2a86c6dd773be545e142e250650f55439d5de4ae",
        ),
    )
}


def validate_skill_request(
    request: AnalyzeRequest,
) -> tuple[SkillContract, StructuredPayload] | None:
    schema_name = request.payload.get("schema_name")
    if schema_name is None:
        return None
    try:
        payload = StructuredPayload.model_validate(request.payload)
    except ValidationError as error:
        raise SkillContractError from error
    contract = _CONTRACTS.get(payload.schema_name)
    if (
        contract is None
        or contract.task_type != request.task_type
        or contract.schema_version != payload.schema_version
        or _canonical_sha256(payload.input_schema) != contract.input_schema_sha256
        or _canonical_sha256(payload.schema_definition) != contract.schema_sha256
    ):
        raise SkillContractError
    try:
        Draft202012Validator.check_schema(payload.input_schema)
        Draft202012Validator.check_schema(payload.schema_definition)
        Draft202012Validator(payload.input_schema, format_checker=FormatChecker()).validate(
            payload.input
        )
    except (SchemaError, JSONSchemaValidationError) as error:
        raise SkillContractError from error
    return contract, payload


def validate_analysis_response(
    request: AnalyzeRequest,
    response: AnalyzeResponse,
    selected_skill: tuple[SkillContract, StructuredPayload] | None,
) -> None:
    if (
        response.contract_version != request.contract_version
        or response.task_id != request.task_id
        or response.task_type != request.task_type
        or (response.status == "succeeded") == response.runtime.degraded
    ):
        raise SkillOutputError
    trusted_evidence_ids = {item.id for item in request.evidence}
    if any(
        suggestion.kind != request.task_type
        or not set(suggestion.evidence_ids).issubset(trusted_evidence_ids)
        for suggestion in response.suggestions
    ):
        raise SkillOutputError
    if selected_skill is None:
        return
    if len(response.suggestions) != 1:
        raise SkillOutputError
    _, payload = selected_skill
    try:
        Draft202012Validator(payload.schema_definition, format_checker=FormatChecker()).validate(
            response.suggestions[0].value
        )
    except JSONSchemaValidationError as error:
        raise SkillOutputError from error


def _canonical_sha256(value: dict[str, Any]) -> str:
    encoded = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(encoded).hexdigest()
