from datetime import date, datetime
from uuid import UUID

from sqlalchemy import (
    CheckConstraint,
    Date,
    DateTime,
    ForeignKey,
    Index,
    Integer,
    String,
    UniqueConstraint,
    text,
)
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
        CheckConstraint(
            "retention_days BETWEEN 1 AND 365", name="ck_collection_runs_retention_days"
        ),
        CheckConstraint("trigger IN ('manual', 'scheduled')", name="ck_collection_runs_trigger"),
        CheckConstraint(
            "(trigger = 'manual' AND schedule_slot IS NULL) OR "
            "(trigger = 'scheduled' AND schedule_slot IS NOT NULL)",
            name="ck_collection_runs_schedule_slot",
        ),
        CheckConstraint("reserved_requests = 1", name="ck_collection_runs_reserved_requests"),
        CheckConstraint(
            "operation IN ('search_posts', 'fetch_post')",
            name="ck_collection_runs_operation",
        ),
        CheckConstraint(
            "(operation = 'search_posts' AND parent_run_id IS NULL) OR "
            "(operation = 'fetch_post' AND parent_run_id IS NOT NULL)",
            name="ck_collection_runs_parent_operation",
        ),
        CheckConstraint(
            "parent_run_id IS NULL OR parent_run_id <> id",
            name="ck_collection_runs_parent_not_self",
        ),
        UniqueConstraint(
            "parent_run_id",
            "operation",
            "request_value",
            name="uq_collection_runs_parent_request",
        ),
        Index(
            "uq_collection_runs_schedule_slot",
            "monitor_version_id",
            "source",
            "operation",
            "request_value",
            "schedule_slot",
            unique=True,
            postgresql_where=text("parent_run_id IS NULL"),
        ),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    job_id: Mapped[UUID] = mapped_column(ForeignKey("jobs.id"), unique=True)
    parent_run_id: Mapped[UUID | None] = mapped_column(
        ForeignKey("collection_runs.id", ondelete="CASCADE"), index=True
    )
    monitor_version_id: Mapped[UUID] = mapped_column(
        ForeignKey("monitor_versions.id", ondelete="CASCADE"), index=True
    )
    source: Mapped[str] = mapped_column(String(32))
    operation: Mapped[str] = mapped_column(String(32))
    request_value: Mapped[str] = mapped_column(String(100))
    idempotency_key: Mapped[str] = mapped_column(String(128))
    policy_version: Mapped[str] = mapped_column(String(64))
    retention_days: Mapped[int] = mapped_column(Integer)
    trigger: Mapped[str] = mapped_column(String(16))
    schedule_slot: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    budget_day: Mapped[date] = mapped_column(Date)
    reserved_requests: Mapped[int] = mapped_column(Integer)
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


class CollectionBudgetUsage(Base):
    __tablename__ = "collection_budget_usage"
    __table_args__ = (
        UniqueConstraint("monitor_version_id", "budget_day", name="uq_collection_budget_usage_day"),
        CheckConstraint("limit_requests > 0", name="ck_collection_budget_limit"),
        CheckConstraint(
            "reserved_requests BETWEEN 0 AND limit_requests",
            name="ck_collection_budget_reserved",
        ),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    monitor_version_id: Mapped[UUID] = mapped_column(
        ForeignKey("monitor_versions.id", ondelete="CASCADE"), index=True
    )
    budget_day: Mapped[date] = mapped_column(Date)
    limit_requests: Mapped[int] = mapped_column(Integer)
    reserved_requests: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


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
