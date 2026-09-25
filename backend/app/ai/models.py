from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import (
    BigInteger,
    CheckConstraint,
    ForeignKey,
    ForeignKeyConstraint,
    Index,
    LargeBinary,
    String,
    UniqueConstraint,
)
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class AiCall(Base):
    __tablename__ = "ai_calls"
    __table_args__ = (
        UniqueConstraint("owner_id", "id", name="ai_calls_owner_id_key"),
        ForeignKeyConstraint(
            ["owner_id", "job_id"],
            ["jobs.owner_id", "jobs.id"],
            name="ai_calls_owner_job_fkey",
        ),
        CheckConstraint(
            "status IN ('succeeded', 'failed')",
            name="ai_calls_status_check",
        ),
        CheckConstraint(
            "failure_code IS NULL OR failure_code IN "
            "('rate_limited', 'unavailable', 'timeout', 'invalid_output', 'failed')",
            name="ai_calls_failure_code_check",
        ),
        CheckConstraint(
            "(status = 'succeeded' AND failure_code IS NULL) OR "
            "(status = 'failed' AND failure_code IS NOT NULL)",
            name="ai_calls_status_failure_check",
        ),
        CheckConstraint(
            "octet_length(input_fingerprint) = 32",
            name="ai_calls_fingerprint_length_check",
        ),
        CheckConstraint(
            "input_tokens >= 0 AND cached_input_tokens >= 0 AND output_tokens >= 0 "
            "AND reasoning_output_tokens >= 0",
            name="ai_calls_token_usage_check",
        ),
        CheckConstraint("duration_ms >= 0", name="ai_calls_duration_check"),
        Index("ai_calls_owner_created_idx", "owner_id", "created_at"),
    )

    id: Mapped[UUID] = mapped_column(primary_key=True)
    owner_id: Mapped[UUID] = mapped_column(ForeignKey("identity_users.id", ondelete="CASCADE"))
    job_id: Mapped[UUID | None]
    purpose: Mapped[str] = mapped_column(String(128))
    provider: Mapped[str] = mapped_column(String(64))
    model: Mapped[str] = mapped_column(String(128))
    prompt_version: Mapped[str] = mapped_column(String(128))
    input_fingerprint: Mapped[bytes] = mapped_column(LargeBinary(32))
    status: Mapped[str] = mapped_column(String(16))
    failure_code: Mapped[str | None] = mapped_column(String(32))
    input_tokens: Mapped[int] = mapped_column(BigInteger)
    cached_input_tokens: Mapped[int] = mapped_column(BigInteger)
    output_tokens: Mapped[int] = mapped_column(BigInteger)
    reasoning_output_tokens: Mapped[int] = mapped_column(BigInteger)
    duration_ms: Mapped[int] = mapped_column(BigInteger)
    created_at: Mapped[datetime]
