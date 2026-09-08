from datetime import timedelta
from hashlib import sha256
from secrets import compare_digest, token_urlsafe

from argon2 import PasswordHasher
from argon2.exceptions import VerificationError
from sqlalchemy import delete, select
from sqlalchemy.orm import Session, sessionmaker

from audit.services import audit
from core.clock import utcnow
from core.errors import AppError
from identity.models import LoginSession, Owner
from identity.schemas import Principal

hasher = PasswordHasher()
_dummy_hash = hasher.hash(token_urlsafe(32))


def digest(value: str) -> str:
    return sha256(value.encode()).hexdigest()


class IdentityService:
    def __init__(self, factory: sessionmaker[Session]):
        self.factory = factory

    def bootstrap(self, username: str, password: str) -> None:
        from identity.schemas import LoginInput

        data = LoginInput(username=username, password=password)
        with self.factory.begin() as session:
            if session.get(Owner, 1) is not None:
                raise AppError("owner_already_initialized", 409)
            session.add(
                Owner(
                    id=1,
                    username=data.username,
                    password_hash=hasher.hash(data.password.get_secret_value()),
                )
            )
            audit(session, "owner_initialized", "1")

    def login(self, username: str, password: str) -> tuple[str, str]:
        now = utcnow()
        with self.factory.begin() as session:
            owner = session.scalar(select(Owner).where(Owner.id == 1).with_for_update())
            if owner and owner.locked_until and owner.locked_until > now:
                raise AppError("login_throttled", 429)
            valid: bool
            try:
                valid = hasher.verify(owner.password_hash if owner else _dummy_hash, password)
            except VerificationError:
                valid = False
            valid = valid and owner is not None and compare_digest(owner.username, username)
            if not valid:
                if owner:
                    if owner.locked_until and owner.locked_until <= now:
                        owner.failed_logins = 0
                        owner.locked_until = None
                    owner.failed_logins += 1
                    if owner.failed_logins >= 5:
                        owner.locked_until = now + timedelta(minutes=15)
                audit(session, "login_failed", "1")
            else:
                assert owner is not None
                owner.failed_logins = 0
                owner.locked_until = None
                token, csrf = token_urlsafe(32), token_urlsafe(32)
                session.execute(delete(LoginSession).where(LoginSession.expires_at <= now))
                session.add(
                    LoginSession(
                        digest=digest(token),
                        owner_id=1,
                        csrf_digest=digest(csrf),
                        expires_at=now + timedelta(hours=12),
                    )
                )
                audit(session, "login", "1")
        if not valid:
            raise AppError("invalid_credentials", 401)
        return token, csrf

    def authenticate(
        self, token: str | None, csrf: str | None = None, mutate: bool = False
    ) -> Principal:
        if not token or len(token) > 128:
            raise AppError("authentication_required", 401)
        with self.factory() as session:
            login = session.get(LoginSession, digest(token))
            if not login or login.expires_at <= utcnow():
                raise AppError("authentication_required", 401)
            if mutate and (
                not csrf or len(csrf) > 128 or not compare_digest(login.csrf_digest, digest(csrf))
            ):
                raise AppError("csrf_invalid", 403)
            owner = session.get(Owner, login.owner_id)
            if not owner:
                raise AppError("authentication_required", 401)
            return Principal(username=owner.username)

    def logout(self, token: str) -> None:
        with self.factory.begin() as session:
            session.execute(delete(LoginSession).where(LoginSession.digest == digest(token)))
            audit(session, "logout", "1")
