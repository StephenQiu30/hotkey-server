from __future__ import annotations

import base64
import hashlib
import hmac
import json
from datetime import datetime, timezone
from typing import Any, Callable, cast
import secrets

from fastapi import Depends, Header, HTTPException, status
from sqlalchemy import select
from sqlalchemy.orm import Session

from server.app.core.settings import settings
from server.app.db.session import get_session
from server.app.models.user import User


Permission = str

_ROLE_PERMISSIONS: dict[str, set[str]] = {
    "keyword.manage": {"admin"},
    "source.manage": {"admin"},
    "settings.manage": {"admin"},
    "checkRun.manage": {"admin"},
    "report.manage": {"admin"},
    "task.manage": {"admin"},
    "readOnly": {"admin", "viewer"},
}


def _normalize_role(role: str | None) -> str:
    normalized = (role or "").strip().lower()
    return normalized or "admin"


def _is_role_permitted(permission: str, role: str) -> bool:
    allowed = _ROLE_PERMISSIONS.get(permission, {"admin"})
    return role in allowed


def _b64url_encode(payload: str) -> str:
    token = base64.urlsafe_b64encode(payload.encode("utf-8")).decode("ascii").rstrip("=")
    return token


def _b64url_decode(token: str) -> bytes:
    normalized = token + "=" * ((4 - len(token) % 4) % 4)
    return base64.urlsafe_b64decode(normalized.encode("ascii"))


def _sign(token: str, secret: str) -> str:
    return hmac.new(secret.encode("utf-8"), token.encode("ascii"), hashlib.sha256).hexdigest()


def _current_timestamp() -> int:
    return int(datetime.now(tz=timezone.utc).timestamp())


def issue_signed_token(payload: dict[str, Any], *, ttl_seconds: int) -> str:
    body = dict(payload)
    body["iat"] = _current_timestamp()
    body["exp"] = _current_timestamp() + ttl_seconds
    raw_json = json.dumps(body, ensure_ascii=False, separators=(",", ":"), sort_keys=True)
    encoded_payload = _b64url_encode(raw_json)
    signature = _sign(encoded_payload, settings.jwt_secret_key)
    return f"{encoded_payload}.{signature}"


def verify_signed_token(token: str, expected_type: str | None = None) -> dict[str, Any]:
    try:
        encoded_payload, signature = token.split(".", 1)
    except ValueError as exc:
        raise HTTPException(status_code=401, detail="Invalid token format.") from exc

    expected_signature = _sign(encoded_payload, settings.jwt_secret_key)
    if not hmac.compare_digest(signature, expected_signature):
        raise HTTPException(status_code=401, detail="Invalid token signature.")

    try:
        payload_bytes = _b64url_decode(encoded_payload)
        payload = json.loads(payload_bytes.decode("utf-8"))
    except (ValueError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise HTTPException(status_code=401, detail="Invalid token payload.") from exc

    if expected_type is not None and payload.get("type") != expected_type:
        raise HTTPException(status_code=401, detail="Invalid token type.")

    now = _current_timestamp()
    exp = cast(int, payload.get("exp"))
    if not isinstance(exp, int) or exp < now:
        raise HTTPException(status_code=401, detail="Token expired.")

    return payload


def issue_github_oauth_state_token() -> str:
    return issue_signed_token(
        {
            "type": "github_oauth_state",
            "state_id": secrets.token_hex(16),
        },
        ttl_seconds=settings.oauth_state_ttl_seconds,
    )


def parse_oauth_state_token(token: str) -> None:
    verify_signed_token(token, expected_type="github_oauth_state")


_PASSWORD_HASH_ALGORITHM = "pbkdf2_sha256"
_PASSWORD_HASH_ITERATIONS = 260_000


def hash_password(password: str) -> str:
    salt = secrets.token_hex(16)
    digest = hashlib.pbkdf2_hmac(
        "sha256",
        password.encode("utf-8"),
        salt.encode("ascii"),
        _PASSWORD_HASH_ITERATIONS,
    ).hex()
    return f"{_PASSWORD_HASH_ALGORITHM}${_PASSWORD_HASH_ITERATIONS}${salt}${digest}"


def verify_password(password: str, password_hash: str | None) -> bool:
    if not password_hash:
        return False
    try:
        algorithm, iterations_raw, salt, expected_digest = password_hash.split("$", 3)
        iterations = int(iterations_raw)
    except ValueError:
        return False
    if algorithm != _PASSWORD_HASH_ALGORITHM or iterations <= 0:
        return False
    digest = hashlib.pbkdf2_hmac(
        "sha256",
        password.encode("utf-8"),
        salt.encode("ascii"),
        iterations,
    ).hex()
    return hmac.compare_digest(digest, expected_digest)


def _user_login_identifier(user: User) -> str:
    if user.email:
        return user.email
    if user.github_login:
        return user.github_login
    if user.platform_provider and user.platform_openid:
        return f"{user.platform_provider}:{user.platform_openid}"
    return str(user.id)


def issue_session_token(user: User) -> str:
    return issue_signed_token(
        {
            "type": "session",
            "sub": str(user.id),
            "login": _user_login_identifier(user),
        },
        ttl_seconds=settings.jwt_session_expire_days * 24 * 60 * 60,
    )


def parse_session_token(token: str) -> int:
    payload = verify_signed_token(token, expected_type="session")
    subject = payload.get("sub")
    if not isinstance(subject, str) or not subject.isdigit():
        raise HTTPException(status_code=401, detail="Invalid session token.")
    return int(subject)


def extract_bearer_token(authorization: str | None) -> str | None:
    if not authorization:
        return None
    if not authorization.startswith("Bearer "):
        return None
    token = authorization.removeprefix("Bearer ").strip()
    return token or None


def get_current_user(
    authorization: str | None = Header(default=None, alias="Authorization"),
    session: Session = Depends(get_session),
) -> User:
    token = extract_bearer_token(authorization)
    if not token:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="Missing access token.")

    user_id = parse_session_token(token)
    user = session.scalar(select(User).where(User.id == user_id))
    if user is None or not user.is_active:
        raise HTTPException(status_code=status.HTTP_401_UNAUTHORIZED, detail="User not found or inactive.")
    return user


def require_permission(permission: Permission) -> Callable[[User], User]:
    if permission not in _ROLE_PERMISSIONS:
        raise ValueError(f"Undefined permission: {permission}")

    def dependency(user: User = Depends(get_current_user)) -> User:
        role = _normalize_role(user.role)
        if not _is_role_permitted(permission, role):
            raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="Insufficient permissions.")
        return user

    return dependency
