from datetime import datetime
from uuid import UUID

from sqlalchemy import CheckConstraint, DateTime, ForeignKey, Integer, String, UniqueConstraint
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class CollectionRun(Base):
    __tablename__ = "collection_runs"
    __table_args__ = (
        UniqueConstraint("idempotency_key", name="uq_collection_runs_idempotency_key"),
        CheckConstraint(
            "state IN ('queued', 'running', 'completed', 'failed', 'cancelled')",
            name="ck_collection_runs_state",
        ),
        CheckConstraint(
            "outcome IS NULL OR outcome IN ('ok', 'empty', 'partial', 'failed')",
            name="ck_collection_runs_outcome",
        ),
        CheckConstraint(
            "fencing_token >= 0 AND pages_count >= 0 AND items_count >= 0 AND bytes_count >= 0",
            name="ck_collection_runs_counts",
        ),
        CheckConstraint("window_since < window_until", name="ck_collection_runs_window"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    monitor_version_id: Mapped[UUID] = mapped_column(
        ForeignKey("monitor_versions.id", ondelete="CASCADE"), index=True
    )
    source: Mapped[str] = mapped_column(String(32))
    operation: Mapped[str] = mapped_column(String(32))
    query_variant: Mapped[str] = mapped_column(String(100))
    idempotency_key: Mapped[str] = mapped_column(String(128))
    policy_version: Mapped[str] = mapped_column(String(64))
    state: Mapped[str] = mapped_column(String(16))
    outcome: Mapped[str | None] = mapped_column(String(16))
    fencing_token: Mapped[int] = mapped_column(Integer)
    window_since: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    window_until: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    pages_count: Mapped[int] = mapped_column(Integer)
    items_count: Mapped[int] = mapped_column(Integer)
    bytes_count: Mapped[int] = mapped_column(Integer)
    stop_reason: Mapped[str | None] = mapped_column(String(80))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    started_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    completed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))


class CollectionCheckpoint(Base):
    __tablename__ = "collection_checkpoints"
    __table_args__ = (
        UniqueConstraint("run_id", "page_key", name="uq_collection_checkpoints_page"),
        CheckConstraint("item_count >= 0", name="ck_collection_checkpoints_item_count"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    run_id: Mapped[UUID] = mapped_column(
        ForeignKey("collection_runs.id", ondelete="CASCADE"), index=True
    )
    page_key: Mapped[str] = mapped_column(String(128))
    cursor: Mapped[str | None] = mapped_column(String(2048))
    raw_page_id: Mapped[UUID] = mapped_column(ForeignKey("raw_pages.id"))
    item_count: Mapped[int] = mapped_column(Integer)
    committed_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
