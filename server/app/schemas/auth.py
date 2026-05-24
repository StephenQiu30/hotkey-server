from __future__ import annotations

from datetime import datetime
from pydantic import BaseModel, ConfigDict, Field


class UserRead(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    github_id: int | None = None
    github_login: str | None = None
    github_name: str | None
    email: str | None
    role: str | None = None
    display_name: str | None = None
    platform_provider: str | None = None
    platform_openid: str | None = None
    avatar_url: str | None
    is_active: bool
    last_login_at: datetime | None
    created_at: datetime
    updated_at: datetime


class AuthResponse(BaseModel):
    access_token: str
    token_type: str = "bearer"
    user: UserRead


class TokenRefreshResponse(AuthResponse):
    pass


class GitHubAuthInitResponse(BaseModel):
    authorization_url: str


class EmailRegisterRequest(BaseModel):
    email: str = Field(min_length=3, max_length=320)
    password: str = Field(min_length=8, max_length=128)
    display_name: str | None = Field(default=None, max_length=80)


class EmailLoginRequest(BaseModel):
    email: str = Field(min_length=3, max_length=320)
    password: str = Field(min_length=1, max_length=128)


class MiniappLoginRequest(BaseModel):
    provider: str = Field(min_length=2, max_length=40)
    openid: str = Field(min_length=1, max_length=128)
    display_name: str | None = Field(default=None, max_length=80)
    avatar_url: str | None = Field(default=None, max_length=500)
