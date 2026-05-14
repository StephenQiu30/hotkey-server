from __future__ import annotations

from urllib.parse import quote_plus, urlencode

import httpx
from fastapi import APIRouter, Depends, HTTPException, Query, status
from fastapi.responses import RedirectResponse
from sqlalchemy import select
from sqlalchemy.orm import Session
from datetime import datetime, timezone

from apps.api.app.core.security import (
    get_current_user,
    issue_github_oauth_state_token,
    issue_session_token,
    parse_oauth_state_token,
)
from apps.api.app.core.settings import settings
from apps.api.app.db.session import get_session
from apps.api.app.models.user import User
from apps.api.app.schemas.auth import AuthResponse, GitHubAuthInitResponse, UserRead

router = APIRouter(prefix="/api/auth", tags=["auth"])


@router.get("/github/login", response_model=GitHubAuthInitResponse)
def github_login() -> GitHubAuthInitResponse:
    if not settings.github_client_id or not settings.github_client_secret:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="GitHub OAuth credentials are not configured.",
        )

    state = issue_github_oauth_state_token()
    redirect_uri = settings.github_redirect_uri or f"{settings.web_base_url.rstrip('/')}/api/auth/github/callback"
    query = urlencode(
        {
            "client_id": settings.github_client_id,
            "redirect_uri": redirect_uri,
            "scope": "read:user user:email",
            "state": state,
        }
    )
    return GitHubAuthInitResponse(
        authorization_url=f"https://github.com/login/oauth/authorize?{query}",
    )


@router.get("/github/callback")
def github_callback(
    code: str | None = Query(default=None),
    state: str | None = Query(default=None),
    session: Session = Depends(get_session),
) -> RedirectResponse:
    if code is None:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="OAuth code is required.")
    if state is None:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="OAuth state is required.")
    parse_oauth_state_token(state)

    if not settings.github_client_id or not settings.github_client_secret:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="GitHub OAuth credentials are not configured.",
        )

    token_response = httpx.post(
        "https://github.com/login/oauth/access_token",
        data={
            "client_id": settings.github_client_id,
            "client_secret": settings.github_client_secret,
            "code": code,
            "redirect_uri": settings.github_redirect_uri or f"{settings.web_base_url.rstrip('/')}/api/auth/github/callback",
        },
        headers={"Accept": "application/json"},
        timeout=15.0,
    )
    if token_response.status_code != status.HTTP_200_OK:
        raise HTTPException(status_code=502, detail="Failed to exchange OAuth token.")
    token_payload = token_response.json()
    if "access_token" not in token_payload:
        raise HTTPException(status_code=400, detail=token_payload.get("error_description", "OAuth exchange failed."))

    access_token = token_payload["access_token"]
    user_response = httpx.get(
        "https://api.github.com/user",
        headers={
            "Authorization": f"Bearer {access_token}",
            "Accept": "application/vnd.github+json",
            "User-Agent": "ai-hotspot-radar",
        },
        timeout=15.0,
    )
    if user_response.status_code != status.HTTP_200_OK:
        raise HTTPException(status_code=502, detail="Failed to fetch GitHub user info.")
    user_payload = user_response.json()

    github_id = user_payload.get("id")
    if not isinstance(github_id, int):
        raise HTTPException(status_code=400, detail="Invalid GitHub user payload.")

    github_email = user_payload.get("email")
    if not github_email:
        email_response = httpx.get(
            "https://api.github.com/user/emails",
            headers={
                "Authorization": f"Bearer {access_token}",
                "Accept": "application/vnd.github+json",
                "User-Agent": "ai-hotspot-radar",
            },
            timeout=15.0,
        )
        if email_response.status_code == status.HTTP_200_OK:
            emails = email_response.json() if isinstance(email_response.json(), list) else []
            for candidate in emails:
                if isinstance(candidate, dict) and candidate.get("primary") and candidate.get("email"):
                    github_email = candidate["email"]
                    break
            if not github_email:
                for candidate in emails:
                    if isinstance(candidate, dict) and candidate.get("email"):
                        github_email = candidate["email"]
                        break

    user = session.scalar(select(User).where(User.github_id == github_id))
    if user is None:
        user = User(
            github_id=github_id,
            github_login=user_payload.get("login", ""),
            github_name=user_payload.get("name"),
            email=github_email,
            avatar_url=user_payload.get("avatar_url"),
            is_active=True,
            last_login_at=datetime.now(tz=timezone.utc),
        )
        session.add(user)
        session.commit()
        session.refresh(user)
    else:
        user.github_login = str(user_payload.get("login", ""))
        user.github_name = user_payload.get("name")
        user.avatar_url = user_payload.get("avatar_url")
        user.email = github_email
        user.is_active = True
        user.last_login_at = datetime.now(tz=timezone.utc)
        session.commit()
        session.refresh(user)

    token = issue_session_token(user)
    callback = f"{settings.web_base_url.rstrip('/')}/auth/github/callback"
    redirect_url = f"{callback}?token={quote_plus(token)}"
    return RedirectResponse(url=redirect_url, status_code=status.HTTP_302_FOUND)


@router.get("/me", response_model=UserRead)
def auth_me(user: User = Depends(get_current_user)) -> User:
    return user


@router.get("/token", response_model=AuthResponse, response_model_exclude_none=True)
def auth_token_exchange(user: User = Depends(get_current_user)) -> AuthResponse:
    token = issue_session_token(user)
    return AuthResponse(access_token=token, user=user)
