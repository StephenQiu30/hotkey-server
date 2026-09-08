from uuid import UUID, uuid4

from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from audit.services import audit
from core.clock import utcnow
from core.errors import AppError
from monitors.models import Monitor
from monitors.schemas import MonitorInput, MonitorPage, MonitorUpdate, MonitorView


class MonitorService:
    def __init__(self, factory: sessionmaker[Session]):
        self.factory = factory

    def create_monitor(self, data: MonitorInput) -> MonitorView:
        with self.factory.begin() as session:
            monitor = Monitor(
                id=uuid4(),
                title=data.title,
                keywords=data.keywords,
                sources=list(data.sources),
                version=1,
                created_at=utcnow(),
            )
            session.add(monitor)
            audit(session, "monitor_created", str(monitor.id))
            session.flush()
            return MonitorView.model_validate(monitor)

    def update_monitor(self, identity: UUID, data: MonitorUpdate) -> MonitorView:
        with self.factory.begin() as session:
            monitor = session.scalar(
                select(Monitor).where(Monitor.id == identity).with_for_update()
            )
            if not monitor:
                raise AppError("monitor_not_found", 404)
            if monitor.version != data.version:
                raise AppError("version_conflict", 409)
            monitor.title, monitor.keywords = data.title, data.keywords
            monitor.sources, monitor.version = list(data.sources), monitor.version + 1
            audit(session, "monitor_updated", str(identity))
            return MonitorView.model_validate(monitor)

    def monitors(self, limit: int, cursor: UUID | None) -> MonitorPage:
        with self.factory() as session:
            query = select(Monitor).order_by(Monitor.id)
            if cursor:
                query = query.where(Monitor.id > cursor)
            rows = list(session.scalars(query.limit(limit + 1)))
            return MonitorPage(
                items=[MonitorView.model_validate(v) for v in rows[:limit]],
                next_cursor=rows[limit - 1].id if len(rows) > limit else None,
            )
