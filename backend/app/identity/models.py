from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    CheckConstraint,
    ForeignKey,
    Index,
    LargeBinary,
    SmallInteger,
    String,
    Text,
    text,
)
from sqlalchemy.orm import Mapped, mapped_column, relationship

from db.base import Base


class IdentityUser(Base):
    __tablename__ = "identity_users"
    __table_args__ = (
        CheckConstraint("singleton_key = 1", name="identity_users_singleton_key_check"),
        CheckConstraint("credential_version >= 1", name="identity_users_credential_version_check"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    singleton_key: Mapped[int] = mapped_column(SmallInteger, unique=True, default=1)
    username: Mapped[str] = mapped_column(String(64), unique=True)
    password_hash: Mapped[str] = mapped_column(Text)
    credential_version: Mapped[int] = mapped_column(default=1)
    created_at: Mapped[datetime]
    updated_at: Mapped[datetime]

    sessions: Mapped[list[IdentitySession]] = relationship(
        back_populates="user",
        cascade="all, delete-orphan",
    )


class IdentitySession(Base):
    __tablename__ = "identity_sessions"
    __table_args__ = (
        CheckConstraint(
            "octet_length(token_digest) = 32",
            name="identity_sessions_token_digest_length_check",
        ),
        CheckConstraint(
            "octet_length(csrf_digest) = 32",
            name="identity_sessions_csrf_digest_length_check",
        ),
        CheckConstraint(
            "credential_version >= 1",
            name="identity_sessions_credential_version_check",
        ),
        CheckConstraint(
            "(revoked_at IS NULL AND revoked_reason IS NULL) OR "
            "(revoked_at IS NOT NULL AND revoked_reason IS NOT NULL)",
            name="identity_sessions_revocation_check",
        ),
        Index("identity_sessions_user_id_idx", "user_id"),
        Index(
            "identity_sessions_active_expiry_idx",
            "expires_at",
            postgresql_where=text("revoked_at IS NULL"),
        ),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    user_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    token_digest: Mapped[bytes] = mapped_column(LargeBinary(32), unique=True)
    csrf_digest: Mapped[bytes] = mapped_column(LargeBinary(32))
    credential_version: Mapped[int]
    created_at: Mapped[datetime]
    expires_at: Mapped[datetime]
    revoked_at: Mapped[datetime | None]
    revoked_reason: Mapped[str | None] = mapped_column(String(32))

    user: Mapped[IdentityUser] = relationship(back_populates="sessions")
