from __future__ import annotations

import unicodedata
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID, uuid4

from sqlalchemy import and_, select
from sqlalchemy.orm import Session

from core.errors import ApplicationError
from monitors.models import MonitorTopic, MonitorTopicVersion
from monitors.schemas import (
    MonitorRuleSetView,
    MonitorTopicCreateInput,
    MonitorTopicReadinessStatus,
    MonitorTopicStatus,
    MonitorTopicUpdateInput,
    MonitorTopicView,
)

_MAX_KEYWORDS_PER_GROUP = 50
_MAX_KEYWORD_LENGTH = 100
_MAX_TOPIC_NAME_LENGTH = 80


@dataclass(frozen=True, slots=True)
class NormalizedMonitorRules:
    match_any: tuple[str, ...]
    match_all: tuple[str, ...]
    exclude: tuple[str, ...]


@dataclass(frozen=True, slots=True)
class MonitorRuleMatch:
    matched: bool
    excluded_by: tuple[str, ...]


def _normalize_text(value: str) -> str:
    return " ".join(unicodedata.normalize("NFKC", value).split())


def _comparison_key(value: str) -> str:
    return value.casefold()


def _normalize_group(values: Iterable[str]) -> tuple[tuple[str, ...], frozenset[str]]:
    normalized: list[str] = []
    keys: set[str] = set()
    for value in values:
        item = _normalize_text(value)
        if not item or len(item) > _MAX_KEYWORD_LENGTH:
            raise ApplicationError("invalid_monitor_rules")
        key = _comparison_key(item)
        if key in keys:
            continue
        keys.add(key)
        normalized.append(item)
    if len(normalized) > _MAX_KEYWORDS_PER_GROUP:
        raise ApplicationError("invalid_monitor_rules")
    return tuple(normalized), frozenset(keys)


def normalize_monitor_rules(
    *,
    match_any: Iterable[str],
    match_all: Iterable[str],
    exclude: Iterable[str],
) -> NormalizedMonitorRules:
    normalized_any, any_keys = _normalize_group(match_any)
    normalized_all, all_keys = _normalize_group(match_all)
    normalized_exclude, exclude_keys = _normalize_group(exclude)
    if not normalized_any and not normalized_all:
        raise ApplicationError("invalid_monitor_rules")
    include_keys = any_keys | all_keys
    if any_keys & all_keys or include_keys & exclude_keys:
        raise ApplicationError("keyword_group_conflict")
    return NormalizedMonitorRules(
        match_any=normalized_any,
        match_all=normalized_all,
        exclude=normalized_exclude,
    )


def evaluate_monitor_rules(rules: NormalizedMonitorRules, content: str) -> MonitorRuleMatch:
    comparable_content = _comparison_key(_normalize_text(content))
    excluded_by = tuple(
        keyword for keyword in rules.exclude if _comparison_key(keyword) in comparable_content
    )
    if excluded_by:
        return MonitorRuleMatch(matched=False, excluded_by=excluded_by)
    any_matches = not rules.match_any or any(
        _comparison_key(keyword) in comparable_content for keyword in rules.match_any
    )
    all_matches = not rules.match_all or all(
        _comparison_key(keyword) in comparable_content for keyword in rules.match_all
    )
    return MonitorRuleMatch(matched=any_matches and all_matches, excluded_by=())


class MonitorTopicService:
    def __init__(
        self,
        session: Session,
        *,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def create_topic(
        self,
        *,
        owner_id: UUID,
        command: MonitorTopicCreateInput,
    ) -> MonitorTopicView:
        name = self._normalize_name(command.name)
        rules = self._normalize_command_rules(command)
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            topic = MonitorTopic(
                id=uuid4(),
                owner_id=owner_id,
                name=name,
                status=MonitorTopicStatus.PAUSED.value,
                readiness_status=MonitorTopicReadinessStatus.PENDING_SOURCE_SELECTION.value,
                current_version=1,
                created_at=now,
                updated_at=now,
            )
            version = MonitorTopicVersion(
                topic_id=topic.id,
                version=1,
                created_by=owner_id,
                match_any=list(rules.match_any),
                match_all=list(rules.match_all),
                exclude=list(rules.exclude),
                created_at=now,
            )
            self._session.add(topic)
            self._session.flush()
            self._session.add(version)
            view = self._view(topic, version)
        return view

    def get_topic(self, *, owner_id: UUID, topic_id: UUID) -> MonitorTopicView:
        self._session.rollback()
        with self._session.begin():
            topic = self._find_topic(owner_id=owner_id, topic_id=topic_id)
            version = self._find_version(topic)
            view = self._view(topic, version)
        return view

    def list_topics(
        self,
        *,
        owner_id: UUID,
        include_archived: bool,
        cursor: UUID | None,
        limit: int,
    ) -> tuple[list[MonitorTopicView], str | None]:
        self._session.rollback()
        with self._session.begin():
            statement = (
                select(MonitorTopic, MonitorTopicVersion)
                .join(
                    MonitorTopicVersion,
                    and_(
                        MonitorTopicVersion.topic_id == MonitorTopic.id,
                        MonitorTopicVersion.version == MonitorTopic.current_version,
                    ),
                )
                .where(MonitorTopic.owner_id == owner_id)
                .order_by(MonitorTopic.id)
                .limit(limit + 1)
            )
            if not include_archived:
                statement = statement.where(
                    MonitorTopic.status != MonitorTopicStatus.ARCHIVED.value
                )
            if cursor is not None:
                statement = statement.where(MonitorTopic.id > cursor)
            rows = list(self._session.execute(statement).all())
            has_more = len(rows) > limit
            page_rows = rows[:limit]
            items = [self._view(topic, version) for topic, version in page_rows]
            next_cursor = str(page_rows[-1][0].id) if has_more else None
        return items, next_cursor

    def clone_topic(self, *, owner_id: UUID, topic_id: UUID) -> MonitorTopicView:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            source = self._find_topic(owner_id=owner_id, topic_id=topic_id)
            source_version = self._find_version(source)
            clone = MonitorTopic(
                id=uuid4(),
                owner_id=owner_id,
                name=source.name,
                status=MonitorTopicStatus.PAUSED.value,
                readiness_status=MonitorTopicReadinessStatus.PENDING_SOURCE_SELECTION.value,
                current_version=1,
                created_at=now,
                updated_at=now,
            )
            clone_version = MonitorTopicVersion(
                topic_id=clone.id,
                version=1,
                created_by=owner_id,
                match_any=list(source_version.match_any),
                match_all=list(source_version.match_all),
                exclude=list(source_version.exclude),
                created_at=now,
            )
            self._session.add(clone)
            self._session.flush()
            self._session.add(clone_version)
            view = self._view(clone, clone_version)
        return view

    def pause_topic(self, *, owner_id: UUID, topic_id: UUID) -> MonitorTopicView:
        return self._transition_topic(
            owner_id=owner_id,
            topic_id=topic_id,
            target=MonitorTopicStatus.PAUSED,
        )

    def resume_topic(self, *, owner_id: UUID, topic_id: UUID) -> MonitorTopicView:
        return self._transition_topic(
            owner_id=owner_id,
            topic_id=topic_id,
            target=MonitorTopicStatus.ACTIVE,
        )

    def archive_topic(self, *, owner_id: UUID, topic_id: UUID) -> MonitorTopicView:
        return self._transition_topic(
            owner_id=owner_id,
            topic_id=topic_id,
            target=MonitorTopicStatus.ARCHIVED,
        )

    def update_topic(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        command: MonitorTopicUpdateInput,
    ) -> MonitorTopicView:
        name = self._normalize_name(command.name)
        rules = self._normalize_command_rules(command)
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            topic = self._find_topic(
                owner_id=owner_id,
                topic_id=topic_id,
                for_update=True,
            )
            if topic.status == MonitorTopicStatus.ARCHIVED.value:
                raise ApplicationError("topic_archived")
            if topic.current_version != command.expected_version:
                raise ApplicationError("topic_version_conflict")
            current = self._find_version(topic)
            rules_changed = (
                tuple(current.match_any) != rules.match_any
                or tuple(current.match_all) != rules.match_all
                or tuple(current.exclude) != rules.exclude
            )
            name_changed = topic.name != name
            if rules_changed:
                next_version = topic.current_version + 1
                current = MonitorTopicVersion(
                    topic_id=topic.id,
                    version=next_version,
                    created_by=owner_id,
                    match_any=list(rules.match_any),
                    match_all=list(rules.match_all),
                    exclude=list(rules.exclude),
                    created_at=now,
                )
                self._session.add(current)
                topic.current_version = next_version
            if name_changed or rules_changed:
                topic.name = name
                topic.updated_at = now
            view = self._view(topic, current)
        return view

    def _transition_topic(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        target: MonitorTopicStatus,
    ) -> MonitorTopicView:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            topic = self._find_topic(
                owner_id=owner_id,
                topic_id=topic_id,
                for_update=True,
            )
            if topic.status == MonitorTopicStatus.ARCHIVED.value:
                if target != MonitorTopicStatus.ARCHIVED:
                    raise ApplicationError("topic_archived")
            elif target == MonitorTopicStatus.ACTIVE:
                if topic.readiness_status != MonitorTopicReadinessStatus.READY.value:
                    raise ApplicationError("topic_not_ready")
                if topic.status != target.value:
                    topic.status = target.value
                    topic.updated_at = now
            elif topic.status != target.value:
                topic.status = target.value
                topic.updated_at = now
            version = self._find_version(topic)
            view = self._view(topic, version)
        return view

    def _find_topic(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        for_update: bool = False,
    ) -> MonitorTopic:
        statement = select(MonitorTopic).where(
            MonitorTopic.id == topic_id,
            MonitorTopic.owner_id == owner_id,
        )
        if for_update:
            statement = statement.with_for_update()
        topic = self._session.scalar(statement)
        if topic is None:
            raise ApplicationError("resource_not_found")
        return topic

    def _find_version(self, topic: MonitorTopic) -> MonitorTopicVersion:
        version = self._session.get(
            MonitorTopicVersion,
            (topic.id, topic.current_version),
        )
        if version is None:
            raise RuntimeError("monitor topic current version is missing")
        return version

    @staticmethod
    def _normalize_name(value: str) -> str:
        name = _normalize_text(value)
        if not name or len(name) > _MAX_TOPIC_NAME_LENGTH:
            raise ApplicationError("invalid_monitor_rules")
        return name

    @staticmethod
    def _normalize_command_rules(
        command: MonitorTopicCreateInput | MonitorTopicUpdateInput,
    ) -> NormalizedMonitorRules:
        return normalize_monitor_rules(
            match_any=command.match_any,
            match_all=command.match_all,
            exclude=command.exclude,
        )

    @staticmethod
    def _view(topic: MonitorTopic, version: MonitorTopicVersion) -> MonitorTopicView:
        return MonitorTopicView(
            id=topic.id,
            name=topic.name,
            status=MonitorTopicStatus(topic.status),
            readiness_status=MonitorTopicReadinessStatus(topic.readiness_status),
            current_version=topic.current_version,
            rules=MonitorRuleSetView(
                match_any=list(version.match_any),
                match_all=list(version.match_all),
                exclude=list(version.exclude),
            ),
            created_at=topic.created_at.astimezone(UTC),
            updated_at=topic.updated_at.astimezone(UTC),
        )
