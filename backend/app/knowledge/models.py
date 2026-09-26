from __future__ import annotations

from datetime import datetime
from uuid import UUID

from sqlalchemy import CheckConstraint, ForeignKey, String, UniqueConstraint
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class KnowledgeExport(Base):
    __tablename__ = "knowledge_exports"
    __table_args__ = (
        CheckConstraint(
            "object_type IN ('daily', 'weekly', 'event', 'topic', 'post', 'qa')",
            name="knowledge_exports_object_type_check",
        ),
        CheckConstraint("relative_path <> ''", name="knowledge_exports_relative_path_check"),
        CheckConstraint(
            "content_sha256 ~ '^[0-9a-f]{64}$'",
            name="knowledge_exports_content_sha256_check",
        ),
        UniqueConstraint(
            "owner_id", "relative_path", name="knowledge_exports_owner_relative_path_key"
        ),
    )

    owner_id: Mapped[UUID] = mapped_column(
        ForeignKey("identity_users.id", ondelete="CASCADE"), primary_key=True
    )
    object_type: Mapped[str] = mapped_column(String(16), primary_key=True)
    object_id: Mapped[UUID] = mapped_column(primary_key=True)
    relative_path: Mapped[str] = mapped_column(String(512))
    content_sha256: Mapped[str] = mapped_column(String(64))
    exported_at: Mapped[datetime]
