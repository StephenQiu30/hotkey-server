from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from typing import Literal
from uuid import UUID, uuid4

from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from audit.services import audit
from collection.services import (
    CollectionTrendRun,
    collection_coverage_runs,
    collection_run_contexts,
)
from contents.services import (
    ContentReference,
    content_references,
    content_trend_records,
)
from core.clock import utcnow
from core.errors import AppError
from events.models import Event, EventMember, EventRevision
from events.schemas import (
    EventInput,
    EventMemberInput,
    EventMergeInput,
    EventPage,
    EventRevisionView,
    EventSourceTrend,
    EventSplitInput,
    EventTrendBucket,
    EventTrendView,
    EventView,
)
from evidence.services import raw_page_run_ids
from notifications.services import record_event_change


@dataclass(frozen=True)
class EventAnalysisScope:
    id: UUID
    current_revision: int
    member_content_ids: tuple[UUID, ...]


def event_analysis_scope(
    session: Session, identity: UUID, *, lock: bool = False
) -> EventAnalysisScope:
    query = select(Event).where(Event.id == identity)
    event = session.scalar(query.with_for_update() if lock else query)
    if event is None:
        raise AppError("event_not_found", 404)
    members = tuple(
        session.scalars(
            select(EventMember.content_id)
            .where(EventMember.event_id == identity)
            .order_by(EventMember.content_id)
        )
    )
    return EventAnalysisScope(
        id=event.id,
        current_revision=event.current_revision,
        member_content_ids=members,
    )


class EventService:
    def __init__(self, factory: sessionmaker[Session]):
        self.factory = factory

    @staticmethod
    def _event(session: Session, identity: UUID, *, lock: bool = False) -> Event:
        query = select(Event).where(Event.id == identity)
        event = session.scalar(query.with_for_update() if lock else query)
        if event is None:
            raise AppError("event_not_found", 404)
        return event

    @staticmethod
    def _snapshot(session: Session, event: Event) -> dict[str, object]:
        members = list(
            session.scalars(
                select(EventMember.content_id)
                .where(EventMember.event_id == event.id)
                .order_by(EventMember.content_id)
            )
        )
        return {
            "title": event.title,
            "summary": event.summary,
            "status": event.status,
            "member_content_ids": [str(identity) for identity in members],
        }

    @classmethod
    def _revise(
        cls,
        session: Session,
        event: Event,
        change_type: Literal[
            "create",
            "add_member",
            "remove_member",
            "merge_in",
            "merge_out",
            "split_in",
            "split_out",
        ],
        related_event_id: UUID | None = None,
    ) -> None:
        revision = EventRevision(
            id=uuid4(),
            event_id=event.id,
            revision=event.current_revision,
            change_type=change_type,
            related_event_id=related_event_id,
            snapshot=cls._snapshot(session, event),
            created_at=event.updated_at,
        )
        session.add(revision)
        if change_type != "create":
            record_event_change(
                session,
                event_id=event.id,
                change_id=revision.id,
                change_type=change_type,
                created_at=event.updated_at,
            )

    @staticmethod
    def _view(session: Session, event: Event) -> EventView:
        members = list(
            session.scalars(
                select(EventMember)
                .where(EventMember.event_id == event.id)
                .order_by(EventMember.added_at, EventMember.content_id)
            )
        )
        references = content_references(session, [member.content_id for member in members])

        def member_view(member: EventMember, content: ContentReference) -> dict[str, object]:
            return {
                "content_id": member.content_id,
                "source": content.source,
                "kind": content.kind,
                "external_id": content.external_id,
                "canonical_url": content.canonical_url,
                "added_at": member.added_at,
            }

        return EventView.model_validate(
            {
                "id": event.id,
                "title": event.title,
                "summary": event.summary,
                "status": event.status,
                "current_revision": event.current_revision,
                "created_at": event.created_at,
                "updated_at": event.updated_at,
                "members": [
                    member_view(member, references[member.content_id]) for member in members
                ],
            }
        )

    def create(self, data: EventInput) -> EventView:
        now = utcnow()
        with self.factory.begin() as session:
            event = Event(
                id=uuid4(),
                title=data.title,
                summary=data.summary,
                status="active",
                current_revision=1,
                created_at=now,
                updated_at=now,
            )
            session.add(event)
            session.flush()
            self._revise(session, event, "create")
            audit(session, "event_created", str(event.id))
            return self._view(session, event)

    def events(self, limit: int, cursor: UUID | None) -> EventPage:
        with self.factory() as session:
            query = select(Event).order_by(Event.id.desc())
            if cursor is not None:
                query = query.where(Event.id < cursor)
            rows = list(session.scalars(query.limit(limit + 1)))
            return EventPage(
                items=[self._view(session, event) for event in rows[:limit]],
                next_cursor=rows[limit - 1].id if len(rows) > limit else None,
            )

    def event(self, identity: UUID) -> EventView:
        with self.factory() as session:
            return self._view(session, self._event(session, identity))

    def add_member(self, identity: UUID, data: EventMemberInput) -> EventView:
        with self.factory.begin() as session:
            event = self._event(session, identity, lock=True)
            if data.content_id not in content_references(session, [data.content_id], lock=True):
                raise AppError("content_not_found", 404)
            assigned = session.scalar(
                select(EventMember.event_id).where(EventMember.content_id == data.content_id)
            )
            if assigned == event.id:
                return self._view(session, event)
            if assigned is not None:
                raise AppError("content_already_assigned", 409)
            now = utcnow()
            session.add(EventMember(event_id=event.id, content_id=data.content_id, added_at=now))
            session.flush()
            event.current_revision += 1
            event.updated_at = now
            self._revise(session, event, "add_member")
            audit(session, "event_member_added", f"{event.id}:{data.content_id}")
            return self._view(session, event)

    def remove_member(self, identity: UUID, content_id: UUID) -> EventView:
        with self.factory.begin() as session:
            event = self._event(session, identity, lock=True)
            member = session.scalar(
                select(EventMember).where(
                    EventMember.event_id == event.id,
                    EventMember.content_id == content_id,
                )
            )
            if member is None:
                raise AppError("event_member_not_found", 404)
            session.delete(member)
            session.flush()
            now = utcnow()
            event.current_revision += 1
            event.updated_at = now
            self._revise(session, event, "remove_member")
            audit(session, "event_member_removed", f"{event.id}:{content_id}")
            return self._view(session, event)

    def revisions(self, identity: UUID) -> list[EventRevisionView]:
        with self.factory() as session:
            self._event(session, identity)
            rows = session.scalars(
                select(EventRevision)
                .where(EventRevision.event_id == identity)
                .order_by(EventRevision.revision)
            )
            return [
                EventRevisionView.model_validate(
                    {
                        "revision": row.revision,
                        "change_type": row.change_type,
                        "related_event_id": row.related_event_id,
                        "snapshot": row.snapshot,
                        "created_at": row.created_at,
                    }
                )
                for row in rows
            ]

    @staticmethod
    def _bucket_index(moment: datetime, since: datetime, bucket: timedelta) -> int:
        return int((moment.astimezone(UTC) - since) // bucket)

    @staticmethod
    def _coverage_reasons(live_runs: list[CollectionTrendRun], policy_changed: bool) -> list[str]:
        reasons: list[str] = []
        if live_runs and policy_changed:
            reasons.append("policy_changed")
        if any(run.state == "failed" or run.outcome == "failed" for run in live_runs):
            reasons.append("run_failed")
        if any(run.state == "cancelled" or run.outcome == "partial" for run in live_runs):
            reasons.append("run_partial")
        if any(
            run.state not in {"completed", "failed", "cancelled"}
            or (run.state == "completed" and run.outcome not in {"ok", "empty", "partial"})
            for run in live_runs
        ):
            reasons.append("run_incomplete")
        if not live_runs:
            reasons.append("no_live_coverage")
        return reasons

    def trends(
        self,
        identity: UUID,
        since: datetime,
        until: datetime,
        bucket_hours: Literal[1, 6, 24],
    ) -> EventTrendView:
        since = since.astimezone(UTC)
        until = until.astimezone(UTC)
        if since >= until:
            raise AppError("invalid_trend_window", 422)
        bucket = timedelta(hours=bucket_hours)
        bucket_count = (until - since + bucket - timedelta(microseconds=1)) // bucket
        if bucket_count > 168:
            raise AppError("trend_window_too_large", 422)

        with self.factory() as session:
            event = self._event(session, identity)
            member_ids = list(
                session.scalars(
                    select(EventMember.content_id).where(EventMember.event_id == event.id)
                )
            )
            records = content_trend_records(session, member_ids, until)
            raw_page_ids = {
                raw_page_id
                for record in records
                for raw_page_id in (
                    [record.initial_raw_page_id] if record.initial_raw_page_id is not None else []
                )
            }
            raw_page_ids.update(
                observation.raw_page_id for record in records for observation in record.observations
            )
            raw_to_run = raw_page_run_ids(session, raw_page_ids)
            run_contexts = collection_run_contexts(session, set(raw_to_run.values()))
            sources = {record.source for record in records}
            coverage = collection_coverage_runs(session, sources, since, until)

        def mode(raw_page_id: UUID | None) -> str | None:
            if raw_page_id is None:
                return None
            run_id = raw_to_run.get(raw_page_id)
            context = run_contexts.get(run_id) if run_id is not None else None
            return context.ingestion_mode if context is not None else None

        result_sources: list[EventSourceTrend] = []
        for source in sorted(sources):
            bucket_data = [
                {
                    "new_posts": 0,
                    "new_discussions": 0,
                    "observed_reply_delta": 0,
                    "excluded_backfill_items": 0,
                    "excluded_backfill_observations": 0,
                }
                for _ in range(bucket_count)
            ]
            for record in (item for item in records if item.source == source):
                if since <= record.first_seen_at < until:
                    index = self._bucket_index(record.first_seen_at, since, bucket)
                    initial_mode = mode(record.initial_raw_page_id)
                    if initial_mode == "live":
                        field = "new_posts" if record.kind == "post" else "new_discussions"
                        bucket_data[index][field] += 1
                    elif initial_mode == "backfill":
                        bucket_data[index]["excluded_backfill_items"] += 1
                previous_live_count: int | None = None
                for observation in record.observations:
                    observation_mode = mode(observation.raw_page_id)
                    in_window = since <= observation.observed_at < until
                    if observation_mode == "backfill":
                        if in_window:
                            index = self._bucket_index(observation.observed_at, since, bucket)
                            bucket_data[index]["excluded_backfill_observations"] += 1
                        continue
                    if observation_mode != "live" or observation.reply_count is None:
                        continue
                    if in_window and previous_live_count is not None:
                        index = self._bucket_index(observation.observed_at, since, bucket)
                        bucket_data[index]["observed_reply_delta"] += (
                            observation.reply_count - previous_live_count
                        )
                    previous_live_count = observation.reply_count

            source_coverage = [run for run in coverage if run.source == source]
            live_policies = {
                run.policy_version for run in source_coverage if run.ingestion_mode == "live"
            }
            policy_changed = len(live_policies) > 1
            buckets: list[EventTrendBucket] = []
            for index, data in enumerate(bucket_data):
                starts_at = since + index * bucket
                ends_at = min(starts_at + bucket, until)
                runs = [
                    run
                    for run in source_coverage
                    if run.window_since.astimezone(UTC) < ends_at
                    and run.window_until.astimezone(UTC) > starts_at
                ]
                live_runs = [run for run in runs if run.ingestion_mode == "live"]
                backfill_runs = [run for run in runs if run.ingestion_mode == "backfill"]
                reasons = self._coverage_reasons(live_runs, policy_changed)
                successful = any(
                    run.state == "completed" and run.outcome in {"ok", "empty"} for run in live_runs
                )
                if not live_runs:
                    coverage_status = "missing"
                elif reasons or not successful:
                    coverage_status = "interrupted"
                else:
                    coverage_status = "comparable"
                buckets.append(
                    EventTrendBucket.model_validate(
                        {
                            "starts_at": starts_at,
                            "ends_at": ends_at,
                            **data,
                            "coverage_status": coverage_status,
                            "interruption_reasons": reasons,
                            "live_run_count": len(live_runs),
                            "backfill_run_count": len(backfill_runs),
                            "policy_versions": sorted({run.policy_version for run in live_runs}),
                        }
                    )
                )
            result_sources.append(EventSourceTrend(source=source, buckets=buckets))
        return EventTrendView(
            event_id=identity,
            since=since,
            until=until,
            bucket_hours=bucket_hours,
            sources=result_sources,
        )

    def merge(self, target_id: UUID, data: EventMergeInput) -> EventView:
        if target_id == data.source_event_id:
            raise AppError("event_merge_same_event", 422)
        with self.factory.begin() as session:
            identities = sorted((target_id, data.source_event_id))
            locked = list(
                session.scalars(
                    select(Event)
                    .where(Event.id.in_(identities))
                    .order_by(Event.id)
                    .with_for_update()
                )
            )
            by_id = {event.id: event for event in locked}
            if len(by_id) != 2:
                raise AppError("event_not_found", 404)
            target = by_id[target_id]
            source = by_id[data.source_event_id]
            if (
                target.current_revision != data.expected_target_revision
                or source.current_revision != data.expected_source_revision
            ):
                raise AppError("version_conflict", 409)
            if target.status != "active" or source.status != "active":
                raise AppError("event_not_active", 409)
            members = list(
                session.scalars(
                    select(EventMember).where(EventMember.event_id == source.id).with_for_update()
                )
            )
            now = utcnow()
            for member in members:
                member.event_id = target.id
            session.flush()
            target.current_revision += 1
            target.updated_at = now
            source.current_revision += 1
            source.updated_at = now
            source.status = "archived"
            self._revise(session, target, "merge_in", source.id)
            self._revise(session, source, "merge_out", target.id)
            audit(session, "event_merged", f"{source.id}:{target.id}")
            return self._view(session, target)

    def split(self, source_id: UUID, data: EventSplitInput) -> EventView:
        with self.factory.begin() as session:
            source = self._event(session, source_id, lock=True)
            if source.current_revision != data.expected_revision:
                raise AppError("version_conflict", 409)
            if source.status != "active":
                raise AppError("event_not_active", 409)
            members = list(
                session.scalars(
                    select(EventMember)
                    .where(EventMember.event_id == source.id)
                    .order_by(EventMember.content_id)
                    .with_for_update()
                )
            )
            selected = set(data.content_ids)
            current = {member.content_id for member in members}
            if not selected.issubset(current):
                raise AppError("event_member_not_found", 404)
            if selected == current:
                raise AppError("event_split_requires_remaining_member", 409)
            now = utcnow()
            created = Event(
                id=uuid4(),
                title=data.title,
                summary=data.summary,
                status="active",
                current_revision=1,
                created_at=now,
                updated_at=now,
            )
            session.add(created)
            session.flush()
            for member in members:
                if member.content_id in selected:
                    member.event_id = created.id
            session.flush()
            source.current_revision += 1
            source.updated_at = now
            self._revise(session, source, "split_out", created.id)
            self._revise(session, created, "split_in", source.id)
            audit(session, "event_split", f"{source.id}:{created.id}")
            return self._view(session, created)
