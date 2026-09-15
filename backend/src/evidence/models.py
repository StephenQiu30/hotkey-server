from datetime import datetime
from uuid import UUID

from sqlalchemy import CheckConstraint, DateTime, ForeignKey, Integer, String, UniqueConstraint
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class RawPage(Base):
    __tablename__ = "raw_pages"
    __table_args__ = (
        UniqueConstraint("object_key", name="uq_raw_pages_object_key"),
        UniqueConstraint(
            "run_id", "request_fingerprint", "payload_sha256", name="uq_raw_pages_response"
        ),
        CheckConstraint("response_bytes >= 0 AND object_bytes >= 0", name="ck_raw_pages_bytes"),
        CheckConstraint("retention_until > observed_at", name="ck_raw_pages_retention"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    run_id: Mapped[UUID] = mapped_column(
        ForeignKey("collection_runs.id", ondelete="CASCADE"), index=True
    )
    source: Mapped[str] = mapped_column(String(32))
    operation: Mapped[str] = mapped_column(String(32))
    request_fingerprint: Mapped[str] = mapped_column(String(64))
    bucket: Mapped[str] = mapped_column(String(255))
    object_key: Mapped[str] = mapped_column(String(1024))
    payload_sha256: Mapped[str] = mapped_column(String(64))
    object_sha256: Mapped[str] = mapped_column(String(64))
    response_bytes: Mapped[int] = mapped_column(Integer)
    object_bytes: Mapped[int] = mapped_column(Integer)
    media_type: Mapped[str] = mapped_column(String(64))
    observed_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    retention_until: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    policy_version: Mapped[str] = mapped_column(String(64))
