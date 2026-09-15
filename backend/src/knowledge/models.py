from datetime import datetime
from uuid import UUID

from pgvector.sqlalchemy import Vector
from sqlalchemy import (
    CheckConstraint,
    DateTime,
    ForeignKey,
    Index,
    Integer,
    String,
    Text,
    UniqueConstraint,
)
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class KnowledgeEntry(Base):
    __tablename__ = "knowledge_entries"
    __table_args__ = (
        CheckConstraint("entry_type IN ('analysis_snapshot')", name="ck_knowledge_entries_type"),
        CheckConstraint("current_version > 0", name="ck_knowledge_entries_current_version"),
        UniqueConstraint("source_analysis_run_id", name="uq_knowledge_entries_analysis_run"),
        Index("ix_knowledge_entries_event_created", "event_id", "created_at"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    event_id: Mapped[UUID] = mapped_column(ForeignKey("events.id", ondelete="RESTRICT"))
    source_analysis_run_id: Mapped[UUID] = mapped_column(
        ForeignKey("analysis_runs.id", ondelete="RESTRICT")
    )
    entry_type: Mapped[str] = mapped_column(String(32))
    current_version: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


class KnowledgeVersion(Base):
    __tablename__ = "knowledge_versions"
    __table_args__ = (
        CheckConstraint("version > 0", name="ck_knowledge_versions_version"),
        UniqueConstraint("entry_id", "version", name="uq_knowledge_versions_entry_version"),
        UniqueConstraint("entry_id", "input_sha256", name="uq_knowledge_versions_entry_input"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    entry_id: Mapped[UUID] = mapped_column(
        ForeignKey("knowledge_entries.id", ondelete="CASCADE"), index=True
    )
    version: Mapped[int] = mapped_column(Integer)
    input_sha256: Mapped[str] = mapped_column(String(64))
    analysis_manifest_sha256: Mapped[str] = mapped_column(String(64))
    title: Mapped[str] = mapped_column(String(240))
    body: Mapped[str] = mapped_column(Text)
    search_text: Mapped[str] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))


class KnowledgeCitation(Base):
    __tablename__ = "knowledge_citations"
    __table_args__ = (
        CheckConstraint("position > 0", name="ck_knowledge_citations_position"),
        UniqueConstraint("version_id", "position", name="uq_knowledge_citations_position"),
        UniqueConstraint(
            "version_id", "content_version_id", name="uq_knowledge_citations_content_version"
        ),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    version_id: Mapped[UUID] = mapped_column(
        ForeignKey("knowledge_versions.id", ondelete="CASCADE"), index=True
    )
    content_version_id: Mapped[UUID] = mapped_column(
        ForeignKey("content_versions.id", ondelete="RESTRICT")
    )
    text_sha256: Mapped[str] = mapped_column(String(64))
    position: Mapped[int] = mapped_column(Integer)


class KnowledgeChunk(Base):
    __tablename__ = "knowledge_chunks"
    __table_args__ = (
        CheckConstraint("position = 1", name="ck_knowledge_chunks_position"),
        CheckConstraint(
            "index_state IN ('pending', 'ready', 'failed', 'stale', 'deleted')",
            name="ck_knowledge_chunks_index_state",
        ),
        CheckConstraint(
            "embedding_model_digest IS NULL OR embedding_model_digest ~ '^[0-9a-f]{64}$'",
            name="ck_knowledge_chunks_model_digest",
        ),
        CheckConstraint(
            "(index_state = 'ready' AND embedding IS NOT NULL AND embedding_model IS NOT NULL "
            "AND embedding_model_digest IS NOT NULL AND embedding_dimensions = 1024 "
            "AND indexed_at IS NOT NULL AND error_code IS NULL) "
            "OR (index_state <> 'ready' AND embedding IS NULL)",
            name="ck_knowledge_chunks_ready_fields",
        ),
        UniqueConstraint("version_id", name="uq_knowledge_chunks_version"),
        Index("ix_knowledge_chunks_state_model", "index_state", "embedding_model"),
    )
    id: Mapped[UUID] = mapped_column(primary_key=True)
    version_id: Mapped[UUID] = mapped_column(
        ForeignKey("knowledge_versions.id", ondelete="CASCADE")
    )
    position: Mapped[int] = mapped_column(Integer)
    text: Mapped[str] = mapped_column(Text)
    text_sha256: Mapped[str] = mapped_column(String(64))
    embedding: Mapped[list[float] | None] = mapped_column(Vector(1024), nullable=True)
    embedding_model: Mapped[str | None] = mapped_column(String(120), nullable=True)
    embedding_model_digest: Mapped[str | None] = mapped_column(String(64), nullable=True)
    embedding_dimensions: Mapped[int | None] = mapped_column(Integer, nullable=True)
    index_state: Mapped[str] = mapped_column(String(16))
    error_code: Mapped[str | None] = mapped_column(String(64), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    indexed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
