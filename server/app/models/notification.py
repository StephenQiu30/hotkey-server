from __future__ import annotations

from datetime import datetime
from typing import TYPE_CHECKING

from sqlalchemy import DateTime, ForeignKey, Text, func
from sqlalchemy.orm import Mapped, mapped_column, relationship

from server.app.db.base import Base

if TYPE_CHECKING:
    from server.app.models.hotspot import Hotspot
    from server.app.models.report import Report


class Notification(Base):
    __tablename__ = "notifications"

    id: Mapped[int] = mapped_column(primary_key=True)
    hotspot_id: Mapped[int | None] = mapped_column(ForeignKey("hotspots.id", ondelete="SET NULL"))
    report_id: Mapped[int | None] = mapped_column(ForeignKey("reports.id", ondelete="SET NULL"))
    channel: Mapped[str] = mapped_column(Text, nullable=False, server_default="email")
    recipient: Mapped[str | None] = mapped_column(Text)
    status: Mapped[str] = mapped_column(Text, nullable=False, server_default="pending")
    error_message: Mapped[str | None] = mapped_column(Text)
    sent_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), nullable=False, server_default=func.now())

    hotspot: Mapped[Hotspot | None] = relationship(back_populates="notifications")
    report: Mapped[Report | None] = relationship(back_populates="notifications")
