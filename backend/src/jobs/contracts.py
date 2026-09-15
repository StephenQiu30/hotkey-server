from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field

JobKind = Literal["verify_pipeline", "collect_page"]


class Dispatch(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)
    job_id: UUID
    epoch: int = Field(ge=1, strict=True)
    contract_version: Literal[1] = 1


class Lease(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)
    job_id: UUID
    epoch: int = Field(ge=1, strict=True)
    fencing_token: int = Field(ge=1, strict=True)
    kind: JobKind = "verify_pipeline"
