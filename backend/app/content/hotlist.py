from __future__ import annotations

import hashlib
from collections.abc import Callable, Iterable
from datetime import UTC, datetime
from typing import Literal
from uuid import UUID, uuid4, uuid5

from sqlalchemy import select
from sqlalchemy.orm import Session

from connections.schemas import SourceEntryPoint
from connections.services import (
    list_applied_hotlist_presets_in_transaction,
    require_source_connection_version,
)
from content.discovery import KeywordDiscoveryPageCommitService, KeywordRequestMeter
from content.models import HotlistEntryRecord, HotlistSnapshot
from content.schemas import (
    HotlistEntryView,
    HotlistSnapshotView,
    HotlistSourceView,
    PersistContentPostInput,
)
from content.services import ContentService
from core.errors import ApplicationError
from evidence.schemas import DataClass
from evidence.services import SourceAccessPolicyService
from jobs.execution import ExecutionLease, JobExecutionService, JobProgress
from jobs.schemas import JobStage
from jobs.services import ResourceBudgetService
from monitors.services import ActiveHotlistTopic, MonitorScheduleService, evaluate_monitor_rules
from sources.contracts import (
    HotlistEntry,
    HotlistPage,
    SourceCapability,
    SourcePageState,
    SourcePost,
)


def recover_hotlist_usage_in_transaction(
    session: Session,
    *,
    owner_id: UUID,
    operation_id: UUID,
    source_key: str,
    finished_at: datetime,
) -> tuple[UUID, ...]:
    return ResourceBudgetService(
        session, clock=lambda: finished_at
    ).recover_abandoned_attempts_in_transaction(
        owner_id=owner_id,
        operation_id=operation_id,
        component_key=f"collector.{source_key}",
        stage="hotlist.request",
        finished_at=finished_at,
    )


def rank_change(rank: int, previous_rank: int | None) -> Literal["new", "up", "down", "same"]:
    if previous_rank is None:
        return "new"
    if rank < previous_rank:
        return "up"
    if rank > previous_rank:
        return "down"
    return "same"


def match_hotlist_topics(
    entry: HotlistEntry, topics: Iterable[ActiveHotlistTopic]
) -> tuple[str, ...]:
    return tuple(topic.name for topic in matching_hotlist_topics(entry, topics))


def matching_hotlist_topics(
    entry: HotlistEntry, topics: Iterable[ActiveHotlistTopic]
) -> tuple[ActiveHotlistTopic, ...]:
    text = "\n".join(part for part in (entry.title, entry.summary) if part)
    return tuple(topic for topic in topics if evaluate_monitor_rules(topic.rules, text).matched)


def _post_payload(entry: HotlistEntry, source_key: str) -> dict[str, object]:
    external_id = entry.url
    if len(external_id) > 512:
        external_id = "sha256:" + hashlib.sha256(external_id.encode()).hexdigest()
    post = SourcePost(
        source_key=source_key,
        external_id=external_id,
        author_external_id=None,
        # Ranking is an observation; source publication time remains on the snapshot entry.
        published_at=None,
        text=entry.summary,
        text_scope="truncated" if entry.summary else None,
        title=entry.title,
        canonical_url=entry.url,
        like_count=None,
        comment_count=None,
        repost_count=None,
    )
    return KeywordDiscoveryPageCommitService._payload(post)


class HotlistService:
    def __init__(
        self,
        session: Session,
        *,
        lease_seconds: int = 75,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._session = session
        self._lease_seconds = lease_seconds
        self._clock = clock or (lambda: datetime.now(UTC))

    def commit_snapshot(
        self,
        *,
        owner_id: UUID,
        lease: ExecutionLease,
        operation_id: UUID,
        source_key: str,
        connection_id: UUID,
        connection_version: int,
        page: HotlistPage,
        meter: KeywordRequestMeter,
    ) -> ExecutionLease:
        if page.source_key != source_key or page.state is not SourcePageState.COMPLETE:
            raise ValueError("only a complete matching hotlist page may be saved")
        self._session.rollback()
        with self._session.begin():
            execution = JobExecutionService(
                self._session, lease_seconds=self._lease_seconds, clock=self._clock
            )
            current = execution.require_current_operation_in_transaction(
                lease, owner_id=owner_id, operation_id=operation_id
            )
            existing = self._session.scalar(
                select(HotlistSnapshot).where(
                    HotlistSnapshot.owner_id == owner_id,
                    HotlistSnapshot.job_id == lease.job_id,
                )
            )
            if existing is not None:
                return current
            require_source_connection_version(
                self._session,
                owner_id=owner_id,
                source_key=source_key,
                connection_id=connection_id,
                connection_version=connection_version,
            )
            policy = SourceAccessPolicyService(self._session, clock=self._clock)
            policy.require_admission_ready_in_transaction(
                owner_id=owner_id,
                source_key=source_key,
                capability=SourceCapability.HOTLIST,
                data_class=DataClass.STRUCTURED,
            )
            topics = MonitorScheduleService(
                self._session
            ).list_active_hotlist_topics_in_transaction(owner_id=owner_id)
            snapshot = HotlistSnapshot(
                id=uuid4(),
                owner_id=owner_id,
                source_key=source_key,
                job_id=lease.job_id,
                observed_at=page.observed_at,
                entry_count=len(page.items),
            )
            self._session.add(snapshot)
            self._session.flush()
            content = ContentService(self._session, clock=self._clock)
            for entry in page.items:
                matches = matching_hotlist_topics(entry, topics)
                names = tuple(topic.name for topic in matches)
                content_id: UUID | None = None
                if names:
                    admitted = policy.admit_payload_in_transaction(
                        owner_id=owner_id,
                        source_key=source_key,
                        capability=SourceCapability.HOTLIST,
                        data_class=DataClass.STRUCTURED,
                        collected_at=page.observed_at,
                        payload=_post_payload(entry, source_key),
                    )
                    persisted = content.persist_post_in_transaction(
                        owner_id=owner_id,
                        command=PersistContentPostInput(
                            job_id=lease.job_id,
                            source_operation_id=uuid5(operation_id, entry.url),
                            connection_id=connection_id,
                            connection_version=connection_version,
                            entry_point=SourceEntryPoint.SCHEDULED,
                            component_name=f"collector.{source_key}",
                            component_version=page.adapter_version,
                            admission=admitted,
                        ),
                    )
                    content_id = persisted.id
                self._session.add(
                    HotlistEntryRecord(
                        snapshot_id=snapshot.id,
                        owner_id=owner_id,
                        rank=entry.rank,
                        title=entry.title,
                        url=entry.url,
                        summary=entry.summary,
                        heat=entry.heat,
                        published_at=entry.published_at,
                        content_id=content_id,
                        matched_topic_names=list(names),
                        matched_topic_ids=[str(topic.topic_id) for topic in matches],
                    )
                )
            meter.settle_page_in_transaction(
                page=page, owner_id=owner_id, lease=lease, operation_id=operation_id
            )
            renewed = execution.save_checkpoint_in_transaction(
                lease,
                sequence=lease.checkpoint_sequence + 1,
                checkpoint={"snapshot_id": str(snapshot.id)},
                progress=JobProgress(stage=JobStage.SAVE, items_saved=len(page.items)),
            )
        meter.confirm_page(renewed)
        return renewed

    def list_sources(self, *, owner_id: UUID) -> tuple[HotlistSourceView, ...]:
        self._session.rollback()
        with self._session.begin():
            applied = list_applied_hotlist_presets_in_transaction(self._session, owner_id=owner_id)
            return tuple(
                HotlistSourceView(
                    source_key=item.source_key,
                    latest_observed_at=self._session.scalar(
                        select(HotlistSnapshot.observed_at)
                        .where(
                            HotlistSnapshot.owner_id == owner_id,
                            HotlistSnapshot.source_key == item.source_key,
                        )
                        .order_by(HotlistSnapshot.observed_at.desc(), HotlistSnapshot.id.desc())
                        .limit(1)
                    ),
                )
                for item in applied
            )

    def get_latest(
        self,
        *,
        owner_id: UUID,
        source_key: str,
        cursor: int | None = None,
        limit: int = 20,
    ) -> HotlistSnapshotView:
        if cursor is not None and cursor < 1:
            raise ApplicationError("resource_not_found")
        self._session.rollback()
        with self._session.begin():
            applied = list_applied_hotlist_presets_in_transaction(self._session, owner_id=owner_id)
            if source_key not in {item.source_key for item in applied}:
                raise ApplicationError("resource_not_found")
            snapshots = self._session.scalars(
                select(HotlistSnapshot)
                .where(
                    HotlistSnapshot.owner_id == owner_id,
                    HotlistSnapshot.source_key == source_key,
                )
                .order_by(HotlistSnapshot.observed_at.desc(), HotlistSnapshot.id.desc())
                .limit(2)
            ).all()
            if not snapshots:
                raise ApplicationError("resource_not_found")
            latest = snapshots[0]
            previous = snapshots[1] if len(snapshots) > 1 else None
            previous_ranks = (
                {
                    row.url: row.rank
                    for row in self._session.scalars(
                        select(HotlistEntryRecord).where(
                            HotlistEntryRecord.snapshot_id == previous.id
                        )
                    )
                }
                if previous is not None
                else {}
            )
            query = select(HotlistEntryRecord).where(HotlistEntryRecord.snapshot_id == latest.id)
            if cursor is not None:
                query = query.where(HotlistEntryRecord.rank > cursor)
            entries = self._session.scalars(
                query.order_by(HotlistEntryRecord.rank).limit(limit + 1)
            ).all()
            selected = entries[:limit]
            return HotlistSnapshotView(
                snapshot_id=latest.id,
                source_key=source_key,
                observed_at=latest.observed_at,
                entry_count=latest.entry_count,
                items=tuple(
                    HotlistEntryView(
                        rank=item.rank,
                        title=item.title,
                        url=item.url,
                        summary=item.summary,
                        heat=item.heat,
                        published_at=item.published_at,
                        content_id=item.content_id,
                        rank_change=rank_change(item.rank, previous_ranks.get(item.url)),
                        matched=bool(item.matched_topic_names),
                        matched_topic_names=tuple(item.matched_topic_names),
                    )
                    for item in selected
                ),
                next_cursor=selected[-1].rank if len(entries) > limit else None,
            )
