from __future__ import annotations

import hashlib
import secrets
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

from pwdlib import PasswordHash
from sqlalchemy import func, select, update
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session, joinedload

from core.config import Settings
from core.errors import ApplicationError
from identity.models import IdentitySession, IdentityUser
from identity.schemas import IdentitySessionView, IdentityUserView, IdentityWorkspaceView

_PASSWORD_HASH = PasswordHash.recommended()
_DUMMY_PASSWORD_HASH = _PASSWORD_HASH.hash("dummy password used only to equalize failed login")
_MIN_PASSWORD_LENGTH = 12
_MAX_PASSWORD_LENGTH = 128


@dataclass(frozen=True, slots=True)
class CreatedIdentitySession:
    view: IdentitySessionView
    session_token: str
    csrf_token: str


@dataclass(frozen=True, slots=True)
class AuthenticatedIdentity:
    session_id: UUID
    view: IdentitySessionView
    csrf_digest: bytes


def _digest(value: str) -> bytes:
    return hashlib.sha256(value.encode()).digest()


def _now() -> datetime:
    return datetime.now(UTC)


def _ensure_password_policy(password: str) -> None:
    if not _MIN_PASSWORD_LENGTH <= len(password) <= _MAX_PASSWORD_LENGTH:
        raise ApplicationError("invalid_password")


def require_resource_owner(actor_id: UUID, owner_id: UUID) -> None:
    if actor_id != owner_id:
        raise ApplicationError("resource_not_found")


class IdentityService:
    def __init__(self, session: Session, settings: Settings) -> None:
        self._session = session
        self._settings = settings

    @property
    def cookie_secure(self) -> bool:
        return self._settings.environment in {"staging", "production"}

    @property
    def session_ttl_seconds(self) -> int:
        return self._settings.session_ttl_seconds

    def initialize_owner(
        self,
        *,
        username: str,
        password: str,
        bootstrap_token: str | None,
    ) -> CreatedIdentitySession:
        _ensure_password_policy(password)
        configured_token = self._settings.bootstrap_token
        if (
            configured_token is None
            or bootstrap_token is None
            or not secrets.compare_digest(
                configured_token.get_secret_value(),
                bootstrap_token,
            )
        ):
            raise ApplicationError("bootstrap_forbidden")

        now = _now()
        user = IdentityUser(
            id=uuid4(),
            username=username,
            password_hash=_PASSWORD_HASH.hash(password),
            credential_version=1,
            created_at=now,
            updated_at=now,
        )
        try:
            with self._session.begin():
                self._session.add(user)
                self._session.flush()
                created = self._new_session(user, now)
        except IntegrityError as error:
            raise ApplicationError("identity_already_initialized") from error
        return created

    def login(self, *, username: str, password: str) -> CreatedIdentitySession:
        now = _now()
        with self._session.begin():
            user = self._session.scalar(
                select(IdentityUser).where(IdentityUser.username == username)
            )
            password_hash = user.password_hash if user is not None else _DUMMY_PASSWORD_HASH
            valid, replacement_hash = _PASSWORD_HASH.verify_and_update(password, password_hash)
            if user is None or not valid:
                raise ApplicationError("invalid_credentials")
            if replacement_hash is not None:
                user.password_hash = replacement_hash
                user.updated_at = now
            return self._new_session(user, now)

    def authenticate(self, session_token: str | None) -> AuthenticatedIdentity:
        if session_token is None:
            raise ApplicationError("invalid_session")
        model = self._session.scalar(
            select(IdentitySession)
            .options(joinedload(IdentitySession.user))
            .where(IdentitySession.token_digest == _digest(session_token))
        )
        now = _now()
        if (
            model is None
            or model.revoked_at is not None
            or model.expires_at <= now
            or model.credential_version != model.user.credential_version
        ):
            self._session.rollback()
            raise ApplicationError("invalid_session")
        return AuthenticatedIdentity(
            session_id=model.id,
            view=self._view(model.user, model.expires_at),
            csrf_digest=model.csrf_digest,
        )

    def validate_csrf(
        self,
        identity: AuthenticatedIdentity,
        *,
        csrf_cookie: str | None,
        csrf_header: str | None,
    ) -> None:
        if (
            csrf_cookie is None
            or csrf_header is None
            or not secrets.compare_digest(csrf_cookie, csrf_header)
            or not secrets.compare_digest(_digest(csrf_header), identity.csrf_digest)
        ):
            raise ApplicationError("csrf_invalid")

    def get_workspace(self, identity: AuthenticatedIdentity) -> IdentityWorkspaceView:
        owner = identity.view.user
        require_resource_owner(identity.view.user.id, owner.id)
        return IdentityWorkspaceView(owner=owner)

    def logout(self, session_id: UUID) -> None:
        now = _now()
        self._session.rollback()
        with self._session.begin():
            model = self._session.get(IdentitySession, session_id, with_for_update=True)
            if model is None or model.revoked_at is not None:
                raise ApplicationError("invalid_session")
            model.revoked_at = now
            model.revoked_reason = "logout"

    def reset_owner_password(self, password: str) -> int:
        _ensure_password_policy(password)
        now = _now()
        self._session.rollback()
        with self._session.begin():
            user = self._session.scalar(select(IdentityUser).with_for_update())
            if user is None:
                raise ApplicationError("identity_not_initialized")
            user.password_hash = _PASSWORD_HASH.hash(password)
            user.credential_version += 1
            user.updated_at = now
            revoked_sessions = self._session.scalar(
                select(func.count())
                .select_from(IdentitySession)
                .where(
                    IdentitySession.user_id == user.id,
                    IdentitySession.revoked_at.is_(None),
                )
            )
            self._session.execute(
                update(IdentitySession)
                .where(
                    IdentitySession.user_id == user.id,
                    IdentitySession.revoked_at.is_(None),
                )
                .values(revoked_at=now, revoked_reason="password_reset")
            )
        return int(revoked_sessions or 0)

    def _new_session(self, user: IdentityUser, now: datetime) -> CreatedIdentitySession:
        session_token = secrets.token_urlsafe(32)
        csrf_token = secrets.token_urlsafe(32)
        expires_at = now + timedelta(seconds=self._settings.session_ttl_seconds)
        model = IdentitySession(
            id=uuid4(),
            user_id=user.id,
            token_digest=_digest(session_token),
            csrf_digest=_digest(csrf_token),
            credential_version=user.credential_version,
            created_at=now,
            expires_at=expires_at,
            revoked_at=None,
            revoked_reason=None,
        )
        self._session.add(model)
        return CreatedIdentitySession(
            view=self._view(user, expires_at),
            session_token=session_token,
            csrf_token=csrf_token,
        )

    @staticmethod
    def _view(user: IdentityUser, expires_at: datetime) -> IdentitySessionView:
        return IdentitySessionView(
            user=IdentityUserView(id=user.id, username=user.username),
            expires_at=expires_at,
        )
