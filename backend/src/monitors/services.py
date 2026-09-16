from datetime import datetime
from typing import Literal, cast
from uuid import UUID, uuid4

from sqlalchemy import Select, and_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from audit.services import audit
from core.clock import utcnow
from core.errors import AppError
from monitors.models import Monitor, MonitorMatch, MonitorVersion
from monitors.schemas import (
    ActiveMonitorConfiguration,
    MonitorInput,
    MonitorMatchReviewInput,
    MonitorMatchReviewState,
    MonitorMatchView,
    MonitorPage,
    MonitorStateChange,
    MonitorUpdate,
    MonitorView,
)
from sources.schemas import QuerySpec, SourceName
from sources.services import SourceService


def match_content(
    session: Session,
    monitor_version_id: UUID,
    content_id: UUID,
    reasons: list[str],
    observed_at: datetime,
) -> None:
    statement = insert(MonitorMatch).values(
        id=uuid4(),
        monitor_version_id=monitor_version_id,
        content_id=content_id,
        match_reason=reasons,
        relevance_status="pending",
        review_state="new",
        first_seen_at=observed_at,
        last_seen_at=observed_at,
    )
    session.execute(
        statement.on_conflict_do_update(
            index_elements=[MonitorMatch.monitor_version_id, MonitorMatch.content_id],
            set_={
                "match_reason": statement.excluded.match_reason,
                "last_seen_at": statement.excluded.last_seen_at,
            },
        )
    )


def _match_view(match: MonitorMatch, version: MonitorVersion) -> MonitorMatchView:
    return MonitorMatchView(
        id=match.id,
        monitor_id=version.monitor_id,
        monitor_version_id=version.id,
        monitor_version=version.version,
        monitor_title=version.title,
        match_reason=match.match_reason,
        relevance_status=cast(
            Literal["pending", "accepted", "rejected", "needs_review"],
            match.relevance_status,
        ),
        review_state=cast(MonitorMatchReviewState, match.review_state),
        first_seen_at=match.first_seen_at,
        last_seen_at=match.last_seen_at,
    )


def monitor_matches_for_content(
    session: Session,
    content_id: UUID,
    *,
    monitor_id: UUID | None = None,
    review_state: MonitorMatchReviewState | None = None,
) -> list[MonitorMatchView]:
    query = (
        select(MonitorMatch, MonitorVersion)
        .join(MonitorVersion, MonitorMatch.monitor_version_id == MonitorVersion.id)
        .where(MonitorMatch.content_id == content_id)
        .order_by(MonitorVersion.title, MonitorVersion.version, MonitorMatch.id)
    )
    if monitor_id is not None:
        query = query.where(MonitorVersion.monitor_id == monitor_id)
    if review_state is None:
        query = query.where(MonitorMatch.review_state != "ignored")
    else:
        query = query.where(MonitorMatch.review_state == review_state)
    return [_match_view(match, version) for match, version in session.execute(query).tuples()]


def matched_content_ids_query(
    *,
    monitor_id: UUID | None = None,
    review_state: MonitorMatchReviewState | None = None,
) -> Select[tuple[UUID]]:
    """Expose a read-only query contract without leaking monitor ORM models."""
    query = select(MonitorMatch.content_id)
    if monitor_id is not None:
        query = query.join(
            MonitorVersion,
            MonitorMatch.monitor_version_id == MonitorVersion.id,
        ).where(MonitorVersion.monitor_id == monitor_id)
    if review_state is None:
        query = query.where(MonitorMatch.review_state != "ignored")
    else:
        query = query.where(MonitorMatch.review_state == review_state)
    return query


def content_is_matched(session: Session, monitor_version_id: UUID, content_id: UUID) -> bool:
    return (
        session.scalar(
            select(MonitorMatch.id).where(
                MonitorMatch.monitor_version_id == monitor_version_id,
                MonitorMatch.content_id == content_id,
            )
        )
        is not None
    )


def monitor_query_spec(session: Session, monitor_version_id: UUID) -> QuerySpec:
    version = session.get(MonitorVersion, monitor_version_id)
    if version is None:
        raise AppError("monitor_version_missing", 500)
    return QuerySpec.model_validate(version.query_spec)


def monitor_version_identity(session: Session, monitor_id: UUID, version: int) -> UUID:
    identity = session.scalar(
        select(MonitorVersion.id).where(
            MonitorVersion.monitor_id == monitor_id,
            MonitorVersion.version == version,
        )
    )
    if identity is None:
        raise AppError("version_conflict", 409)
    return identity


def monitor_version_is_active(session: Session, monitor_version_id: UUID) -> bool:
    return (
        session.scalar(
            select(Monitor.id)
            .join(MonitorVersion, MonitorVersion.monitor_id == Monitor.id)
            .where(
                MonitorVersion.id == monitor_version_id,
                Monitor.current_version == MonitorVersion.version,
                Monitor.state == "active",
            )
        )
        is not None
    )


def active_monitor_configuration(
    session: Session, monitor_id: UUID, expected_version: int
) -> ActiveMonitorConfiguration:
    monitor = session.scalar(select(Monitor).where(Monitor.id == monitor_id).with_for_update())
    if monitor is None:
        raise AppError("monitor_not_found", 404)
    if monitor.current_version != expected_version:
        raise AppError("version_conflict", 409)
    if monitor.state != "active":
        raise AppError("monitor_not_active", 409)
    version = MonitorService._current(session, monitor)
    return ActiveMonitorConfiguration(
        monitor_id=monitor.id,
        monitor_version_id=version.id,
        version=version.version,
        query_spec=QuerySpec.model_validate(version.query_spec),
        source_ids=cast(list[SourceName], version.source_ids),
        schedule=version.schedule,
        budget=version.budget,
    )


def query_match_reasons(query: QuerySpec, text: str) -> list[str]:
    normalized = text.casefold()
    if any(term.casefold() in normalized for term in query.exclude):
        return []
    if any(term.casefold() not in normalized for term in query.include_all):
        return []
    candidates = [*query.include_any, *query.aliases]
    return list(dict.fromkeys(term for term in candidates if term.casefold() in normalized))


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

    def review_match(self, identity: UUID, data: MonitorMatchReviewInput) -> MonitorMatchView:
        with self.factory.begin() as session:
            match = session.get(MonitorMatch, identity, with_for_update=True)
            if match is None:
                raise AppError("monitor_match_not_found", 404)
            version = session.get(MonitorVersion, match.monitor_version_id)
            if version is None:
                raise AppError("monitor_version_missing", 500)
            if match.review_state != data.review_state:
                match.review_state = data.review_state
                audit(session, "monitor_match_reviewed", f"{identity}:{data.review_state}")
                session.flush()
            return _match_view(match, version)

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

    def active_configurations(self) -> list[ActiveMonitorConfiguration]:
        with self.factory() as session:
            rows = session.execute(
                select(Monitor, MonitorVersion)
                .join(
                    MonitorVersion,
                    and_(
                        MonitorVersion.monitor_id == Monitor.id,
                        MonitorVersion.version == Monitor.current_version,
                    ),
                )
                .where(Monitor.state == "active")
                .order_by(Monitor.id)
            ).tuples()
            return [
                ActiveMonitorConfiguration(
                    monitor_id=monitor.id,
                    monitor_version_id=version.id,
                    version=version.version,
                    query_spec=QuerySpec.model_validate(version.query_spec),
                    source_ids=cast(list[SourceName], version.source_ids),
                    schedule=version.schedule,
                    budget=version.budget,
                )
                for monitor, version in rows
            ]
