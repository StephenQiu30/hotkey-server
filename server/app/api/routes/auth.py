from __future__ import annotations

from urllib.parse import quote_plus, urlencode

import httpx
from fastapi import APIRouter, Depends, HTTPException, Query, status
from fastapi.responses import RedirectResponse
from sqlalchemy import select
from sqlalchemy.orm import Session
from datetime import datetime, timezone

from server.app.core.security import (
    get_current_user,
    hash_password,
    issue_github_oauth_state_token,
    issue_session_token,
    parse_oauth_state_token,
    verify_password,
)
from server.app.core.settings import settings
from server.app.db.session import get_session
from server.app.models.user import User
from server.app.schemas.auth import (
    AuthResponse,
    EmailLoginRequest,
    EmailRegisterRequest,
    GitHubAuthInitResponse,
    MiniappLoginRequest,
    TokenRefreshResponse,
    UserRead,
)

router = APIRouter(prefix="/api/auth", tags=["auth"])


def _normalize_email(email: str) -> str:
    return email.strip().lower()


def _normalize_optional_text(value: str | None) -> str | None:
    if value is None:
        return None
    normalized = value.strip()
    return normalized or None


def _auth_response_for(user: User) -> AuthResponse:
    return AuthResponse(access_token=issue_session_token(user), user=user)


@router.post("/register", response_model=AuthResponse, status_code=status.HTTP_201_CREATED)
def register_with_email(payload: EmailRegisterRequest, session: Session = Depends(get_session)) -> AuthResponse:
    email = _normalize_email(payload.email)
    if "@" not in email:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="A valid email is required.")

    existing_user = session.scalar(select(User).where(User.email == email))
    if existing_user is not None:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail="Email is already registered.")

    user = User(
        email=email,
        password_hash=hash_password(payload.password),
        display_name=_normalize_optional_text(payload.display_name) or email.split("@", 1)[0],
        is_active=True,
        last_login_at=datetime.now(tz=timezone.utc),
    )
    session.add(user)
    session.commit()
    session.refresh(user)
    return _auth_response_for(user)


@router.post("/login", response_model=AuthResponse)
def login_with_email(payload: EmailLoginRequest, session: Session = Depends(get_session)) -> AuthResponse:
    email = _normalize_email(payload.email)
    user = session.scalar(select(User).where(User.email == email))
    if user is None or not verify_password(payload.password, user.password_hash):
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Invalid email or password.")
    if not user.is_active:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="User is inactive.")

    user.last_login_at = datetime.now(tz=timezone.utc)
    session.commit()
    session.refresh(user)
    return _auth_response_for(user)


@router.post("/miniapp/login", response_model=AuthResponse)
def login_with_miniapp(payload: MiniappLoginRequest, session: Session = Depends(get_session)) -> AuthResponse:
    provider = payload.provider.strip().lower()
    openid = payload.openid.strip()
    if not provider or not openid:
        raise HTTPException(status_code=status.HTTP_422_UNPROCESSABLE_ENTITY, detail="Miniapp provider and openid are required.")

    user = session.scalar(
        select(User).where(
            User.platform_provider == provider,
            User.platform_openid == openid,
        )
    )
    if user is None:
        user = User(
            platform_provider=provider,
            platform_openid=openid,
            display_name=_normalize_optional_text(payload.display_name) or "小程序用户",
            avatar_url=_normalize_optional_text(payload.avatar_url),
            is_active=True,
            last_login_at=datetime.now(tz=timezone.utc),
        )
        session.add(user)
        session.commit()
        session.refresh(user)
        return _auth_response_for(user)

    user.display_name = _normalize_optional_text(payload.display_name) or user.display_name
    user.avatar_url = _normalize_optional_text(payload.avatar_url) or user.avatar_url
    user.is_active = True
    user.last_login_at = datetime.now(tz=timezone.utc)
    session.commit()
    session.refresh(user)
    return _auth_response_for(user)


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
    return _auth_response_for(user)


@router.post("/token/refresh", response_model=TokenRefreshResponse, response_model_exclude_none=True)
def refresh_auth_token(user: User = Depends(get_current_user)) -> TokenRefreshResponse:
    return TokenRefreshResponse(access_token=issue_session_token(user), user=user)
