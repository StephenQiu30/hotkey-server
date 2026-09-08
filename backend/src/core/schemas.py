from pydantic import BaseModel, ConfigDict


class Input(BaseModel):
    model_config = ConfigDict(extra="forbid", str_strip_whitespace=True, hide_input_in_errors=True)


class ErrorView(BaseModel):
    code: str
    request_id: str


class HealthView(BaseModel):
    status: str
    code: str | None = None
    scope: str | None = None
