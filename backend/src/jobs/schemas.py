from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict

from core.schemas import Input


class JobView(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: UUID
    kind: Literal["verify_pipeline", "collect_page"]
    status: Literal["queued", "running", "succeeded", "failed", "cancelled"]
    attempts: int
    epoch: int
    deadline: datetime


class JobPage(BaseModel):
    items: list[JobView]
    next_cursor: UUID | None


class DiagnosticInput(Input):
    kind: Literal["verify_pipeline"]
