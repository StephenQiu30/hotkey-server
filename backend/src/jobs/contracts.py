from dataclasses import dataclass
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class Dispatch(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)
    job_id: UUID
    epoch: int = Field(ge=1, strict=True)
    contract_version: Literal[1] = 1


@dataclass(frozen=True)
class Lease:
    job_id: UUID
    epoch: int
    fencing_token: int
