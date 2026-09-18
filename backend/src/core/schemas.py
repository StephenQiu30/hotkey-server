from typing import Literal

from pydantic import BaseModel, ConfigDict


class InputModel(BaseModel):
    model_config = ConfigDict(extra="forbid", str_strip_whitespace=True)


class OutputModel(BaseModel):
    model_config = ConfigDict(from_attributes=True)


class HealthView(OutputModel):
    status: Literal["ok", "ready"]


class ErrorView(OutputModel):
    code: str
    message: str
    request_id: str
