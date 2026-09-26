from __future__ import annotations

import unicodedata
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from datetime import UTC, datetime
from uuid import UUID, uuid4

from sqlalchemy import and_, select
from sqlalchemy.orm import Session

from connections.services import load_applied_source_presets_in_transaction
from core.errors import ApplicationError
from monitors.models import (
    FollowedAccount,
    FollowedAccountAlias,
    MonitorSchedule,
    MonitorTopic,
    MonitorTopicVersion,
)
from monitors.schemas import (
    FollowedAccountAliasView,
    FollowedAccountIdentityInput,
    FollowedAccountView,
    MonitorExpansionPreviewView,
    MonitorRulePreviewSampleView,
    MonitorRuleSetView,
    MonitorTopicCreateInput,
    MonitorTopicPreviewInput,
    MonitorTopicPreviewView,
    MonitorTopicReadinessStatus,
    MonitorTopicStatus,
    MonitorTopicUpdateInput,
    MonitorTopicView,
)
from sources.contracts import SourceCapability

_MAX_KEYWORDS_PER_GROUP = 50
_MAX_KEYWORD_LENGTH = 100
_MAX_TOPIC_NAME_LENGTH = 80


def _latest_timestamp(current: datetime, observed: datetime) -> datetime:
    current_utc = current.replace(tzinfo=UTC) if current.tzinfo is None else current.astimezone(UTC)
    observed_utc = (
        observed.replace(tzinfo=UTC) if observed.tzinfo is None else observed.astimezone(UTC)
    )
    return max(current_utc, observed_utc)


@dataclass(frozen=True, slots=True)
class NormalizedMonitorRules:
    match_any: tuple[str, ...]
    match_all: tuple[str, ...]
    exclude: tuple[str, ...]


@dataclass(frozen=True, slots=True)
class MonitorRuleMatch:
    matched: bool
    matched_any: tuple[str, ...]
    matched_all: tuple[str, ...]
    excluded_by: tuple[str, ...]


@dataclass(frozen=True, slots=True)
class DueCollectionSchedule:
    owner_id: UUID
    topic_id: UUID
    topic_version: int
    search_queries: tuple[str, ...]
    source_key: str
    capability: SourceCapability
    interval_seconds: int
    next_run_at: datetime
    last_job_id: UUID | None


@dataclass(frozen=True, slots=True)
class ActiveTopicScan:
    owner_id: UUID
    topic_id: UUID
    topic_version: int
    rules: NormalizedMonitorRules
    source_keys: tuple[str, ...]


@dataclass(frozen=True, slots=True)
class ActiveHotlistTopic:
    owner_id: UUID
    topic_id: UUID
    name: str
    rules: NormalizedMonitorRules


def scheduled_collection_queries(rules: NormalizedMonitorRules) -> tuple[str, ...]:
    """Keep upstream searches portable while local rules retain final relevance semantics."""
    if rules.match_any:
        return rules.match_any
    return (" ".join(rules.match_all),)


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
    matched_any = tuple(
        keyword for keyword in rules.match_any if _comparison_key(keyword) in comparable_content
    )
    matched_all = tuple(
        keyword for keyword in rules.match_all if _comparison_key(keyword) in comparable_content
    )
    excluded_by = tuple(
        keyword for keyword in rules.exclude if _comparison_key(keyword) in comparable_content
    )
    any_matches = not rules.match_any or bool(matched_any)
    all_matches = len(matched_all) == len(rules.match_all)
    return MonitorRuleMatch(
        matched=any_matches and all_matches and not excluded_by,
        matched_any=matched_any,
        matched_all=matched_all,
        excluded_by=excluded_by,
    )


class MonitorScheduleService:
    """Own due-schedule locking and advancement inside scheduler transactions."""

    def __init__(self, session: Session) -> None:
        self._session = session

    def claim_due_collections_in_transaction(
        self,
        *,
        now: datetime,
    ) -> tuple[DueCollectionSchedule, ...]:
        if not self._session.in_transaction():
            raise RuntimeError("collection schedule claims require the caller's transaction")
        if now.tzinfo is None:
            raise ValueError("scheduler time must be timezone-aware")
        rows = self._session.execute(
            select(MonitorSchedule, MonitorTopic, MonitorTopicVersion)
            .join(
                MonitorTopic,
                and_(
                    MonitorTopic.owner_id == MonitorSchedule.owner_id,
                    MonitorTopic.id == MonitorSchedule.topic_id,
                ),
            )
            .join(
                MonitorTopicVersion,
                and_(
                    MonitorTopicVersion.topic_id == MonitorTopic.id,
                    MonitorTopicVersion.version == MonitorTopic.current_version,
                ),
            )
            .where(
                MonitorSchedule.enabled.is_(True),
                MonitorSchedule.next_run_at <= now,
                MonitorSchedule.capability == SourceCapability.SEARCH.value,
            )
            .order_by(
                MonitorSchedule.next_run_at,
                MonitorSchedule.owner_id,
                MonitorSchedule.topic_id,
                MonitorSchedule.source_key,
            )
            .with_for_update(of=MonitorSchedule, skip_locked=True)
        ).all()
        return tuple(
            DueCollectionSchedule(
                owner_id=schedule.owner_id,
                topic_id=schedule.topic_id,
                topic_version=version.version,
                search_queries=scheduled_collection_queries(
                    normalize_monitor_rules(
                        match_any=version.match_any,
                        match_all=version.match_all,
                        exclude=version.exclude,
                    )
                ),
                source_key=schedule.source_key,
                capability=SourceCapability(schedule.capability),
                interval_seconds=schedule.interval_seconds,
                next_run_at=schedule.next_run_at,
                last_job_id=schedule.last_job_id,
            )
            for schedule, topic, version in rows
        )

    def list_active_topics_for_scanning_in_transaction(self) -> tuple[ActiveTopicScan, ...]:
        """Return active topic snapshots without exposing monitor ORM models cross-domain."""
        if not self._session.in_transaction():
            raise RuntimeError("topic scanning requires the caller's transaction")
        rows = self._session.execute(
            select(MonitorTopic, MonitorTopicVersion, MonitorSchedule.source_key)
            .join(
                MonitorTopicVersion,
                and_(
                    MonitorTopicVersion.topic_id == MonitorTopic.id,
                    MonitorTopicVersion.version == MonitorTopic.current_version,
                ),
            )
            .join(
                MonitorSchedule,
                and_(
                    MonitorSchedule.owner_id == MonitorTopic.owner_id,
                    MonitorSchedule.topic_id == MonitorTopic.id,
                ),
            )
            .where(
                MonitorTopic.status == MonitorTopicStatus.ACTIVE.value,
                MonitorTopic.readiness_status == MonitorTopicReadinessStatus.READY.value,
                MonitorSchedule.enabled.is_(True),
                MonitorSchedule.capability == SourceCapability.SEARCH.value,
            )
            .order_by(MonitorTopic.owner_id, MonitorTopic.id, MonitorSchedule.source_key)
        ).all()
        grouped: dict[tuple[UUID, UUID], tuple[MonitorTopicVersion, list[str]]] = {}
        for topic, version, source_key in rows:
            key = (topic.owner_id, topic.id)
            current = grouped.get(key)
            if current is None:
                grouped[key] = (version, [source_key])
            else:
                current[1].append(source_key)
        return tuple(
            ActiveTopicScan(
                owner_id=owner_id,
                topic_id=topic_id,
                topic_version=version.version,
                rules=normalize_monitor_rules(
                    match_any=version.match_any,
                    match_all=version.match_all,
                    exclude=version.exclude,
                ),
                source_keys=tuple(source_keys),
            )
            for (owner_id, topic_id), (version, source_keys) in grouped.items()
        )

    def list_active_hotlist_topics_in_transaction(
        self, *, owner_id: UUID
    ) -> tuple[ActiveHotlistTopic, ...]:
        if not self._session.in_transaction():
            raise RuntimeError("hotlist topic scan requires the caller's transaction")
        rows = self._session.execute(
            select(MonitorTopic, MonitorTopicVersion)
            .join(
                MonitorTopicVersion,
                and_(
                    MonitorTopicVersion.topic_id == MonitorTopic.id,
                    MonitorTopicVersion.version == MonitorTopic.current_version,
                ),
            )
            .where(
                MonitorTopic.owner_id == owner_id,
                MonitorTopic.status == MonitorTopicStatus.ACTIVE.value,
            )
            .order_by(MonitorTopic.id)
        ).all()
        return tuple(
            ActiveHotlistTopic(
                owner_id=owner_id,
                topic_id=topic.id,
                name=topic.name,
                rules=normalize_monitor_rules(
                    match_any=version.match_any,
                    match_all=version.match_all,
                    exclude=version.exclude,
                ),
            )
            for topic, version in rows
        )

    def advance_collection_in_transaction(
        self,
        *,
        schedule: DueCollectionSchedule,
        job_id: UUID,
        next_run_at: datetime,
        updated_at: datetime,
    ) -> None:
        if not self._session.in_transaction():
            raise RuntimeError("collection schedule advancement requires the caller's transaction")
        if next_run_at.tzinfo is None or updated_at.tzinfo is None or next_run_at <= updated_at:
            raise ValueError("next collection run must follow the scheduler time")
        model = self._session.get(
            MonitorSchedule,
            (
                schedule.owner_id,
                schedule.topic_id,
                schedule.source_key,
                schedule.capability.value,
            ),
        )
        if model is None:
            raise RuntimeError("claimed collection schedule is no longer visible")
        model.next_run_at = next_run_at
        model.last_job_id = job_id
        model.updated_at = updated_at


class FollowedAccountService:
    def __init__(
        self,
        session: Session,
        *,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._session = session
        self._clock = clock or (lambda: datetime.now(UTC))

    def record_confirmed_identity(
        self,
        *,
        owner_id: UUID,
        identity: FollowedAccountIdentityInput,
    ) -> FollowedAccountView:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            account = self._session.scalar(
                select(FollowedAccount)
                .where(
                    FollowedAccount.owner_id == owner_id,
                    FollowedAccount.source_key == identity.source_key,
                    FollowedAccount.external_id == identity.external_id,
                )
                .with_for_update()
            )
            if account is None:
                account = FollowedAccount(
                    id=uuid4(),
                    owner_id=owner_id,
                    source_key=identity.source_key,
                    external_id=identity.external_id,
                    display_name=identity.display_name,
                    created_at=now,
                    updated_at=now,
                )
                self._session.add(account)
                self._session.flush()
            else:
                if identity.display_name is not None:
                    account.display_name = identity.display_name
                account.updated_at = _latest_timestamp(account.updated_at, now)

            if identity.alias_value is not None:
                alias = self._session.get(
                    FollowedAccountAlias,
                    (owner_id, account.id, identity.alias_value),
                )
                if alias is None:
                    self._session.add(
                        FollowedAccountAlias(
                            owner_id=owner_id,
                            account_id=account.id,
                            alias_value=identity.alias_value,
                            first_seen_at=now,
                            last_seen_at=now,
                        )
                    )
                else:
                    alias.last_seen_at = _latest_timestamp(alias.last_seen_at, now)

            self._session.flush()
            aliases = self._aliases(owner_id=owner_id, account_id=account.id)
            result = self._view(account, aliases)
        return result

    def get_account(self, *, owner_id: UUID, account_id: UUID) -> FollowedAccountView:
        self._session.rollback()
        with self._session.begin():
            account = self._session.scalar(
                select(FollowedAccount).where(
                    FollowedAccount.owner_id == owner_id,
                    FollowedAccount.id == account_id,
                )
            )
            if account is None:
                raise ApplicationError("resource_not_found")
            aliases = self._aliases(owner_id=owner_id, account_id=account.id)
            result = self._view(account, aliases)
        return result

    def find_by_alias(
        self,
        *,
        owner_id: UUID,
        source_key: str,
        alias_value: str,
    ) -> list[FollowedAccountView]:
        self._session.rollback()
        with self._session.begin():
            accounts = list(
                self._session.scalars(
                    select(FollowedAccount)
                    .join(
                        FollowedAccountAlias,
                        and_(
                            FollowedAccountAlias.owner_id == FollowedAccount.owner_id,
                            FollowedAccountAlias.account_id == FollowedAccount.id,
                        ),
                    )
                    .where(
                        FollowedAccount.owner_id == owner_id,
                        FollowedAccount.source_key == source_key,
                        FollowedAccountAlias.alias_value == alias_value,
                    )
                    .order_by(FollowedAccount.id)
                ).all()
            )
            results = [
                self._view(
                    account,
                    self._aliases(owner_id=owner_id, account_id=account.id),
                )
                for account in accounts
            ]
        return results

    def _aliases(self, *, owner_id: UUID, account_id: UUID) -> list[FollowedAccountAlias]:
        return list(
            self._session.scalars(
                select(FollowedAccountAlias)
                .where(
                    FollowedAccountAlias.owner_id == owner_id,
                    FollowedAccountAlias.account_id == account_id,
                )
                .order_by(FollowedAccountAlias.first_seen_at, FollowedAccountAlias.alias_value)
            ).all()
        )

    @staticmethod
    def _view(
        account: FollowedAccount,
        aliases: list[FollowedAccountAlias],
    ) -> FollowedAccountView:
        latest_alias = max(
            aliases,
            key=lambda alias: (alias.last_seen_at, alias.alias_value),
            default=None,
        )
        return FollowedAccountView(
            id=account.id,
            source_key=account.source_key,
            external_id=account.external_id,
            display_name=account.display_name,
            latest_observed_alias=latest_alias.alias_value if latest_alias else None,
            aliases=[
                FollowedAccountAliasView(
                    alias_value=alias.alias_value,
                    first_seen_at=alias.first_seen_at,
                    last_seen_at=alias.last_seen_at,
                )
                for alias in aliases
            ],
            created_at=account.created_at,
            updated_at=account.updated_at,
        )


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
        source_keys = self._normalize_source_keys(command.source_keys)
        notification_target_names = self._normalize_notification_targets(
            command.notification_target_names
        )
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            self._require_applied_search_sources(owner_id=owner_id, source_keys=source_keys)
            topic = MonitorTopic(
                id=uuid4(),
                owner_id=owner_id,
                name=name,
                status=MonitorTopicStatus.PAUSED.value,
                readiness_status=(
                    MonitorTopicReadinessStatus.READY.value
                    if source_keys
                    else MonitorTopicReadinessStatus.PENDING_SOURCE_SELECTION.value
                ),
                current_version=1,
                collection_interval_seconds=command.collection_interval_seconds,
                report_time=command.report_time,
                report_timezone="Asia/Shanghai",
                weekly_report_enabled=command.weekly_report_enabled,
                notification_target_names=list(notification_target_names),
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
            self._sync_search_schedules(topic=topic, source_keys=source_keys, now=now)
            view = self._view(topic, version, source_keys=source_keys)
        return view

    def preview_topic(self, *, command: MonitorTopicPreviewInput) -> MonitorTopicPreviewView:
        rules = self._normalize_command_rules(command)
        samples = []
        for index, title in enumerate(command.sample_titles):
            result = evaluate_monitor_rules(rules, title)
            samples.append(
                MonitorRulePreviewSampleView(
                    sample_index=index,
                    matched=result.matched,
                    matched_any=list(result.matched_any),
                    matched_all=list(result.matched_all),
                    excluded_by=list(result.excluded_by),
                )
            )
        return MonitorTopicPreviewView(
            rules=MonitorRuleSetView(
                match_any=list(rules.match_any),
                match_all=list(rules.match_all),
                exclude=list(rules.exclude),
            ),
            samples=samples,
            expansion=MonitorExpansionPreviewView(
                local_alias_external_queries=0,
                local_alias_budget_units=0,
                upstream_status="pending_source_selection",
                upstream_external_queries=None,
                upstream_budget_units=None,
            ),
        )

    def get_topic(self, *, owner_id: UUID, topic_id: UUID) -> MonitorTopicView:
        self._session.rollback()
        with self._session.begin():
            topic = self._find_topic(owner_id=owner_id, topic_id=topic_id)
            version = self._find_version(topic)
            view = self._view(topic, version, source_keys=self._source_keys(topic))
        return view

    def get_topic_rules_in_transaction(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        version: int,
    ) -> NormalizedMonitorRules:
        if not self._session.in_transaction():
            raise RuntimeError("topic rules require the caller's transaction")
        topic = self._find_topic(owner_id=owner_id, topic_id=topic_id, for_update=True)
        version_row = self._session.get(MonitorTopicVersion, (topic.id, version))
        if version_row is None:
            raise ApplicationError("resource_not_found")
        return normalize_monitor_rules(
            match_any=version_row.match_any,
            match_all=version_row.match_all,
            exclude=version_row.exclude,
        )

    def get_current_topic_rules_in_transaction(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
    ) -> tuple[int, NormalizedMonitorRules]:
        """Expose the current immutable rule snapshot without leaking monitor ORM models."""
        if not self._session.in_transaction():
            raise RuntimeError("topic rules require the caller's transaction")
        topic = self._find_topic(owner_id=owner_id, topic_id=topic_id)
        version = self._find_version(topic)
        return topic.current_version, normalize_monitor_rules(
            match_any=version.match_any,
            match_all=version.match_all,
            exclude=version.exclude,
        )

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
            items = [
                self._view(topic, version, source_keys=self._source_keys(topic))
                for topic, version in page_rows
            ]
            next_cursor = str(page_rows[-1][0].id) if has_more else None
        return items, next_cursor

    def clone_topic(self, *, owner_id: UUID, topic_id: UUID) -> MonitorTopicView:
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            source = self._find_topic(owner_id=owner_id, topic_id=topic_id)
            source_version = self._find_version(source)
            source_keys = self._source_keys(source)
            self._require_applied_search_sources(owner_id=owner_id, source_keys=source_keys)
            clone = MonitorTopic(
                id=uuid4(),
                owner_id=owner_id,
                name=source.name,
                status=MonitorTopicStatus.PAUSED.value,
                readiness_status=(
                    MonitorTopicReadinessStatus.READY.value
                    if source_keys
                    else MonitorTopicReadinessStatus.PENDING_SOURCE_SELECTION.value
                ),
                current_version=1,
                collection_interval_seconds=source.collection_interval_seconds,
                report_time=source.report_time,
                report_timezone=source.report_timezone,
                weekly_report_enabled=source.weekly_report_enabled,
                notification_target_names=list(source.notification_target_names),
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
            self._sync_search_schedules(topic=clone, source_keys=source_keys, now=now)
            view = self._view(clone, clone_version, source_keys=source_keys)
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
        source_keys = self._normalize_source_keys(command.source_keys)
        notification_target_names = self._normalize_notification_targets(
            command.notification_target_names
        )
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
            self._require_applied_search_sources(owner_id=owner_id, source_keys=source_keys)
            current = self._find_version(topic)
            rules_changed = (
                tuple(current.match_any) != rules.match_any
                or tuple(current.match_all) != rules.match_all
                or tuple(current.exclude) != rules.exclude
            )
            name_changed = topic.name != name
            settings_changed = (
                topic.collection_interval_seconds != command.collection_interval_seconds
                or topic.report_time != command.report_time
                or topic.weekly_report_enabled != command.weekly_report_enabled
                or tuple(topic.notification_target_names) != notification_target_names
                or tuple(self._source_keys(topic)) != source_keys
            )
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
            if name_changed or rules_changed or settings_changed:
                topic.name = name
                if not source_keys and topic.status == MonitorTopicStatus.ACTIVE.value:
                    topic.status = MonitorTopicStatus.PAUSED.value
                topic.readiness_status = (
                    MonitorTopicReadinessStatus.READY.value
                    if source_keys
                    else MonitorTopicReadinessStatus.PENDING_SOURCE_SELECTION.value
                )
                topic.collection_interval_seconds = command.collection_interval_seconds
                topic.report_time = command.report_time
                topic.weekly_report_enabled = command.weekly_report_enabled
                topic.notification_target_names = list(notification_target_names)
                topic.updated_at = now
                self._sync_search_schedules(topic=topic, source_keys=source_keys, now=now)
            view = self._view(topic, current, source_keys=source_keys)
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
                source_keys = self._source_keys(topic)
                if not source_keys:
                    raise ApplicationError("topic_not_ready")
                self._require_applied_search_sources(owner_id=owner_id, source_keys=source_keys)
                topic.readiness_status = MonitorTopicReadinessStatus.READY.value
                if topic.status != target.value:
                    topic.status = target.value
                    topic.updated_at = now
                self._set_schedules_enabled(topic=topic, enabled=True, now=now)
            elif topic.status != target.value:
                topic.status = target.value
                topic.updated_at = now
            if target in {MonitorTopicStatus.PAUSED, MonitorTopicStatus.ARCHIVED}:
                self._set_schedules_enabled(topic=topic, enabled=False, now=now)
            version = self._find_version(topic)
            view = self._view(topic, version, source_keys=self._source_keys(topic))
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

    def _source_keys(self, topic: MonitorTopic) -> tuple[str, ...]:
        return tuple(
            self._session.scalars(
                select(MonitorSchedule.source_key)
                .where(
                    MonitorSchedule.owner_id == topic.owner_id,
                    MonitorSchedule.topic_id == topic.id,
                    MonitorSchedule.capability == SourceCapability.SEARCH.value,
                )
                .order_by(MonitorSchedule.source_key)
            )
        )

    def _require_applied_search_sources(
        self,
        *,
        owner_id: UUID,
        source_keys: tuple[str, ...],
    ) -> None:
        applied = load_applied_source_presets_in_transaction(
            self._session,
            owner_id=owner_id,
            source_keys=source_keys,
        )
        if set(applied) != set(source_keys) or any(
            SourceCapability.SEARCH not in preset.capabilities for preset in applied.values()
        ):
            raise ApplicationError("source_preset_not_applied")

    def _sync_search_schedules(
        self,
        *,
        topic: MonitorTopic,
        source_keys: tuple[str, ...],
        now: datetime,
    ) -> None:
        existing = {
            schedule.source_key: schedule
            for schedule in self._session.scalars(
                select(MonitorSchedule)
                .where(
                    MonitorSchedule.owner_id == topic.owner_id,
                    MonitorSchedule.topic_id == topic.id,
                    MonitorSchedule.capability == SourceCapability.SEARCH.value,
                )
                .with_for_update()
            )
        }
        selected = set(source_keys)
        for source_key, schedule in existing.items():
            if source_key not in selected:
                self._session.delete(schedule)
        for source_key in source_keys:
            existing_schedule = existing.get(source_key)
            if existing_schedule is None:
                self._session.add(
                    MonitorSchedule(
                        owner_id=topic.owner_id,
                        topic_id=topic.id,
                        source_key=source_key,
                        capability=SourceCapability.SEARCH.value,
                        interval_seconds=topic.collection_interval_seconds,
                        next_run_at=now,
                        enabled=topic.status == MonitorTopicStatus.ACTIVE.value,
                        last_job_id=None,
                        created_at=now,
                        updated_at=now,
                    )
                )
                continue
            existing_schedule.interval_seconds = topic.collection_interval_seconds
            existing_schedule.enabled = topic.status == MonitorTopicStatus.ACTIVE.value
            existing_schedule.updated_at = now

    def _set_schedules_enabled(
        self,
        *,
        topic: MonitorTopic,
        enabled: bool,
        now: datetime,
    ) -> None:
        for schedule in self._session.scalars(
            select(MonitorSchedule)
            .where(
                MonitorSchedule.owner_id == topic.owner_id,
                MonitorSchedule.topic_id == topic.id,
            )
            .with_for_update()
        ):
            if schedule.enabled != enabled:
                schedule.enabled = enabled
                schedule.updated_at = now

    @staticmethod
    def _normalize_name(value: str) -> str:
        name = _normalize_text(value)
        if not name or len(name) > _MAX_TOPIC_NAME_LENGTH:
            raise ApplicationError("invalid_monitor_rules")
        return name

    @staticmethod
    def _normalize_command_rules(
        command: MonitorTopicCreateInput | MonitorTopicPreviewInput | MonitorTopicUpdateInput,
    ) -> NormalizedMonitorRules:
        return normalize_monitor_rules(
            match_any=command.match_any,
            match_all=command.match_all,
            exclude=command.exclude,
        )

    @staticmethod
    def _normalize_source_keys(values: Iterable[str]) -> tuple[str, ...]:
        return tuple(sorted(dict.fromkeys(values)))

    @staticmethod
    def _normalize_notification_targets(values: Iterable[str]) -> tuple[str, ...]:
        normalized: list[str] = []
        keys: set[str] = set()
        for value in values:
            item = _normalize_text(value)
            key = _comparison_key(item)
            if key not in keys:
                keys.add(key)
                normalized.append(item)
        return tuple(normalized)

    @staticmethod
    def _view(
        topic: MonitorTopic,
        version: MonitorTopicVersion,
        *,
        source_keys: Iterable[str],
    ) -> MonitorTopicView:
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
            source_keys=list(source_keys),
            collection_interval_seconds=topic.collection_interval_seconds,
            report_time=topic.report_time,
            report_timezone="Asia/Shanghai",
            weekly_report_enabled=topic.weekly_report_enabled,
            notification_target_names=list(topic.notification_target_names),
            created_at=topic.created_at.astimezone(UTC),
            updated_at=topic.updated_at.astimezone(UTC),
        )
