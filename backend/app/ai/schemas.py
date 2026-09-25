from __future__ import annotations

from enum import StrEnum
from typing import Any
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class AiFailureCode(StrEnum):
    RATE_LIMITED = "rate_limited"
    UNAVAILABLE = "unavailable"
    TIMEOUT = "timeout"
    INVALID_OUTPUT = "invalid_output"
    FAILED = "failed"


class AiCallStatus(StrEnum):
    SUCCEEDED = "succeeded"
    FAILED = "failed"


class AiTokenUsage(BaseModel):
    model_config = ConfigDict(frozen=True)

    input_tokens: int = Field(default=0, ge=0)
    cached_input_tokens: int = Field(default=0, ge=0)
    output_tokens: int = Field(default=0, ge=0)
    reasoning_output_tokens: int = Field(default=0, ge=0)


class AiCompletion(BaseModel):
    """A structured model answer; `output` already matches the requested JSON Schema."""

    model_config = ConfigDict(frozen=True)

    provider: str
    model: str
    call_id: UUID | None = None
    output: dict[str, Any]
    usage: AiTokenUsage
    duration_ms: int = Field(ge=0)


class AiCallError(Exception):
    def __init__(self, code: AiFailureCode, detail: str = "") -> None:
        super().__init__(code.value)
        self.code = code
        # Upstream text may echo prompt content; keep it short and out of logs by default.
        self.detail = detail[:500]
