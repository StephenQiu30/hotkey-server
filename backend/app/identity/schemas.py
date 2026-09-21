from __future__ import annotations

from datetime import datetime
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, SecretStr, field_validator


class IdentityCredentialsInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    username: str = Field(
        min_length=3,
        max_length=64,
        pattern=r"^[A-Za-z0-9][A-Za-z0-9._-]*$",
    )
    password: SecretStr = Field(min_length=12, max_length=128)

    @field_validator("username")
    @classmethod
    def normalize_username(cls, value: str) -> str:
        return value.lower()


class IdentityUserView(BaseModel):
    id: UUID
    username: str


class IdentitySessionView(BaseModel):
    user: IdentityUserView
    expires_at: datetime


class IdentityWorkspaceView(BaseModel):
    owner: IdentityUserView
