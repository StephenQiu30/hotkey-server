from typing import Literal, cast
from uuid import UUID, uuid4

from sqlalchemy import and_, select
from sqlalchemy.orm import Session, sessionmaker

from audit.services import audit
from core.clock import utcnow
from core.errors import AppError
from monitors.models import Monitor, MonitorVersion
from monitors.schemas import (
    MonitorInput,
    MonitorPage,
    MonitorStateChange,
    MonitorUpdate,
    MonitorView,
)
from sources.schemas import SourceName
from sources.services import SourceService


class MonitorService:
    def __init__(self, factory: sessionmaker[Session], sources: SourceService | None = None):
        self.factory = factory
        self.sources = sources or SourceService()

    @staticmethod
    def _view(monitor: Monitor, version: MonitorVersion) -> MonitorView:
        return MonitorView.model_validate(
            {
                "id": monitor.id,
                "title": version.title,
                "state": monitor.state,
                "current_version": monitor.current_version,
                "query_spec": version.query_spec,
                "source_ids": version.source_ids,
                "schedule": version.schedule,
                "budget": version.budget,
                "created_at": monitor.created_at,
                "updated_at": monitor.updated_at,
            }
        )

    @staticmethod
    def _snapshot(monitor: Monitor, data: MonitorInput, version: int) -> MonitorVersion:
        return MonitorVersion(
            id=uuid4(),
            monitor_id=monitor.id,
            version=version,
            title=data.title,
            query_spec=data.query_spec.model_dump(),
            source_ids=list(data.source_ids),
            schedule=data.schedule.model_dump(),
            budget=data.budget.model_dump(),
            created_at=utcnow(),
        )

    @staticmethod
    def _current(session: Session, monitor: Monitor) -> MonitorVersion:
        version = session.scalar(
            select(MonitorVersion).where(
                MonitorVersion.monitor_id == monitor.id,
                MonitorVersion.version == monitor.current_version,
            )
        )
        if version is None:
            raise AppError("monitor_version_missing", 500)
        return version

    def create_monitor(self, data: MonitorInput) -> MonitorView:
        now = utcnow()
        with self.factory.begin() as session:
            monitor = Monitor(
                id=uuid4(), state="draft", current_version=1, created_at=now, updated_at=now
            )
            version = self._snapshot(monitor, data, 1)
            session.add_all((monitor, version))
            audit(session, "monitor_created", str(monitor.id))
            session.flush()
            return self._view(monitor, version)

    def update_monitor(self, identity: UUID, data: MonitorUpdate) -> MonitorView:
        with self.factory.begin() as session:
            monitor = session.scalar(
                select(Monitor).where(Monitor.id == identity).with_for_update()
            )
            if monitor is None:
                raise AppError("monitor_not_found", 404)
            if monitor.current_version != data.expected_version:
                raise AppError("version_conflict", 409)
            if monitor.state not in {"draft", "paused"}:
                raise AppError("monitor_not_editable", 409)
            next_version = monitor.current_version + 1
            version = self._snapshot(monitor, data, next_version)
            monitor.current_version = next_version
            monitor.updated_at = utcnow()
            session.add(version)
            audit(session, "monitor_updated", str(identity))
            session.flush()
            return self._view(monitor, version)

    def change_state(
        self, identity: UUID, data: MonitorStateChange, target: Literal["active", "paused"]
    ) -> MonitorView:
        with self.factory.begin() as session:
            monitor = session.scalar(
                select(Monitor).where(Monitor.id == identity).with_for_update()
            )
            if monitor is None:
                raise AppError("monitor_not_found", 404)
            if monitor.current_version != data.expected_version:
                raise AppError("version_conflict", 409)
            version = self._current(session, monitor)
            if target == "active":
                if monitor.state not in {"draft", "paused"}:
                    raise AppError("monitor_state_conflict", 409)
                source_ids = cast(list[SourceName], version.source_ids)
                if self.sources.activation_issues(source_ids):
                    raise AppError("source_not_eligible", 409)
                view = self._view(monitor, version)
                slots_per_day = (1440 + view.schedule.interval_minutes - 1) // (
                    view.schedule.interval_minutes
                )
                required = (
                    self.sources.request_estimate(view.query_spec, source_ids) * slots_per_day
                )
                if required > view.budget.daily_requests:
                    raise AppError("monitor_budget_insufficient", 409)
                action = "monitor_activated"
            else:
                if monitor.state != "active":
                    raise AppError("monitor_state_conflict", 409)
                action = "monitor_paused"
            monitor.state = target
            monitor.updated_at = utcnow()
            audit(session, action, str(identity))
            session.flush()
            return self._view(monitor, version)

    def monitors(self, limit: int, cursor: UUID | None) -> MonitorPage:
        with self.factory() as session:
            query = (
                select(Monitor, MonitorVersion)
                .join(
                    MonitorVersion,
                    and_(
                        MonitorVersion.monitor_id == Monitor.id,
                        MonitorVersion.version == Monitor.current_version,
                    ),
                )
                .order_by(Monitor.id)
            )
            if cursor:
                query = query.where(Monitor.id > cursor)
            rows = list(session.execute(query.limit(limit + 1)).tuples())
            return MonitorPage(
                items=[self._view(monitor, version) for monitor, version in rows[:limit]],
                next_cursor=rows[limit - 1][0].id if len(rows) > limit else None,
            )
