from __future__ import annotations

from datetime import datetime
from pydantic import BaseModel, ConfigDict


class UserRead(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    github_id: int
    github_login: str
    github_name: str | None
    email: str | None
    avatar_url: str | None
    is_active: bool
    last_login_at: datetime | None
    created_at: datetime
    updated_at: datetime


class AuthResponse(BaseModel):
    access_token: str
    token_type: str = "bearer"
    user: UserRead


class GitHubAuthInitResponse(BaseModel):
    authorization_url: str
