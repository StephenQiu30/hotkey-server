from datetime import datetime
from uuid import UUID

from sqlalchemy import CheckConstraint, DateTime, String
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column

from db.base import Base


class Monitor(Base):
    __tablename__ = "monitors"
    __table_args__ = (CheckConstraint("version > 0"),)
    id: Mapped[UUID] = mapped_column(primary_key=True)
    title: Mapped[str] = mapped_column(String(100))
    keywords: Mapped[list[str]] = mapped_column(JSONB)
    sources: Mapped[list[str]] = mapped_column(JSONB)
    version: Mapped[int] = mapped_column(default=1)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
