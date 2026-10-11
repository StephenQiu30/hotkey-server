from __future__ import annotations

import unicodedata
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from itertools import pairwise
from uuid import UUID, uuid4

from sqlalchemy import and_, func, select, true
from sqlalchemy.orm import Session

from connections.services import (
    load_applied_source_presets_in_transaction,
    load_execution_policy_in_transaction,
)
from core.errors import ApplicationError
from evidence.services import load_source_access_readiness
from jobs.schemas import BudgetMetric, BudgetScopeKind
from jobs.services import ResourceBudgetService
from monitors.editorial_events import editorial_event_topic_id, reject_internal_editorial_topic
from monitors.models import (
    FollowedAccount,
    FollowedAccountAlias,
    MonitorSchedule,
    MonitorTopic,
    MonitorTopicStatusEvent,
    MonitorTopicVersion,
)
from monitors.schemas import (
    EditorialTopicSourceView,
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


@dataclass(frozen=True, slots=True)
class ContentTopicReadContext:
    topic_id: UUID
    name: str
    current_version: int


def load_content_topic_contexts_in_transaction(
    session: Session, *, owner_id: UUID, topic_ids: set[UUID]
) -> dict[UUID, ContentTopicReadContext]:
    if not session.in_transaction():
        raise RuntimeError("content topic reads require the caller's transaction")
    if not topic_ids:
        return {}
    topics = session.scalars(
        select(MonitorTopic).where(
            MonitorTopic.owner_id == owner_id, MonitorTopic.id.in_(topic_ids)
        )
    ).all()
    return {
        topic.id: ContentTopicReadContext(
            topic_id=topic.id, name=topic.name, current_version=topic.current_version
        )
        for topic in topics
    }


def current_topic_rule_matches_in_transaction(
    session: Session, *, owner_id: UUID, topic_id: UUID, version: int, require_active: bool = True
) -> bool:
    """Read the current owner rule without admitting a collection or analysis job."""
    if not session.in_transaction():
        raise RuntimeError("topic state read requires caller transaction")
    statement = select(MonitorTopic.id).where(
        MonitorTopic.owner_id == owner_id,
        MonitorTopic.id == topic_id,
        MonitorTopic.current_version == version,
    )
    if require_active:
        statement = statement.where(MonitorTopic.status == MonitorTopicStatus.ACTIVE.value)
    return session.scalar(statement) is not None


def active_collection_sequence_in_transaction(
    session: Session, *, owner_id: UUID, topic_id: UUID
) -> int | None:
    """Read active state and its monotonic sequence in one MVCC snapshot.

    Do not lock the topic here: callers may already hold a Job or Schedule lock,
    while topic writes lock Topic before Schedule. A changed sequence fences any
    acceptance racing with pause, including pause/resume at the same timestamp.
    """
    if not session.in_transaction():
        raise RuntimeError("collection topic reads require caller transaction")
    sequence = (
        select(func.coalesce(func.max(MonitorTopicStatusEvent.event_sequence), 0))
        .where(
            MonitorTopicStatusEvent.owner_id == owner_id,
            MonitorTopicStatusEvent.topic_id == topic_id,
        )
        .scalar_subquery()
    )
    return session.scalar(
        select(sequence).where(
            MonitorTopic.owner_id == owner_id,
            MonitorTopic.id == topic_id,
            MonitorTopic.status == MonitorTopicStatus.ACTIVE.value,
        )
    )


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
class TopicAnalysisRuleTimeline:
    """Immutable rule window and verified active spans, without monitor ORM leakage."""

    rules: NormalizedMonitorRules
    source_keys: tuple[str, ...]
    starts_at: datetime
    ends_at: datetime | None
    active_spans: tuple[tuple[datetime, datetime | None], ...]


@dataclass(frozen=True, slots=True)
class TopicAnalysisRuleIdentity:
    """Owner-scoped immutable rule identity for historical analysis audits."""

    topic_id: UUID
    topic_rule_version: int
    rules: NormalizedMonitorRules
    source_keys: tuple[str, ...]


def list_active_editorial_topic_rules_in_transaction(
    session: Session, *, owner_id: UUID, profile_id: UUID
) -> tuple[TopicAnalysisRuleIdentity, ...]:
    """Freeze selected rules while serializing with pause/change; no source request."""
    if not session.in_transaction():
        raise RuntimeError("editorial topic matching requires the caller's transaction")
    rows = session.execute(
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
            MonitorTopicVersion.editorial_profile_ids.contains([str(profile_id)]),
        )
        .order_by(MonitorTopic.id)
        .with_for_update(of=MonitorTopic)
    ).all()
    return tuple(
        TopicAnalysisRuleIdentity(
            topic_id=topic.id,
            topic_rule_version=version.version,
            rules=normalize_monitor_rules(
                match_any=version.match_any,
                match_all=version.match_all,
                exclude=version.exclude,
            ),
            source_keys=tuple(version.source_keys),
        )
        for topic, version in rows
    )


def list_topic_analysis_rule_identities_in_transaction(
    session: Session, *, owner_id: UUID, before: datetime
) -> tuple[TopicAnalysisRuleIdentity, ...]:
    if not session.in_transaction():
        raise RuntimeError("topic rule reads require the caller's transaction")
    if before.tzinfo is None:
        raise ValueError("topic rule cutoff must be timezone-aware")
    versions = session.scalars(
        select(MonitorTopicVersion)
        .join(MonitorTopic, MonitorTopic.id == MonitorTopicVersion.topic_id)
        .where(
            MonitorTopic.owner_id == owner_id,
            MonitorTopicVersion.created_at < before.astimezone(UTC),
        )
        .order_by(MonitorTopicVersion.topic_id, MonitorTopicVersion.version)
    ).all()
    return tuple(
        TopicAnalysisRuleIdentity(
            topic_id=version.topic_id,
            topic_rule_version=version.version,
            rules=normalize_monitor_rules(
                match_any=version.match_any,
                match_all=version.match_all,
                exclude=version.exclude,
            ),
            source_keys=tuple(version.source_keys),
        )
        for version in versions
    )


def load_topic_analysis_rule_timeline_in_transaction(
    session: Session,
    *,
    owner_id: UUID,
    topic_id: UUID,
    topic_rule_version: int,
) -> TopicAnalysisRuleTimeline | None:
    """Return None when the lifecycle cannot be reconstructed from an initial event."""
    if not session.in_transaction():
        raise RuntimeError("topic timeline reads require the caller's transaction")
    topic = session.scalar(
        select(MonitorTopic).where(MonitorTopic.owner_id == owner_id, MonitorTopic.id == topic_id)
    )
    if topic is None:
        return None
    versions = tuple(
        session.scalars(
            select(MonitorTopicVersion)
            .where(MonitorTopicVersion.topic_id == topic_id)
            .order_by(MonitorTopicVersion.version)
        )
    )
    events = tuple(
        session.scalars(
            select(MonitorTopicStatusEvent)
            .where(
                MonitorTopicStatusEvent.owner_id == owner_id,
                MonitorTopicStatusEvent.topic_id == topic_id,
            )
            .order_by(MonitorTopicStatusEvent.event_sequence)
        )
    )
    if (
        len(versions) != topic.current_version
        or any(row.version != index for index, row in enumerate(versions, 1))
        or any(left.created_at > right.created_at for left, right in pairwise(versions))
        or not 1 <= topic_rule_version <= len(versions)
        or not events
        or events[0].event_sequence != 1
        or events[0].status != MonitorTopicStatus.PAUSED.value
        or events[-1].status != topic.status
    ):
        return None
    previous_status = None
    previous_at = None
    for sequence, event in enumerate(events, 1):
        if (
            event.event_sequence != sequence
            or event.topic_rule_version > topic.current_version
            or (previous_at is not None and event.occurred_at < previous_at)
            or (
                previous_status == MonitorTopicStatus.PAUSED.value
                and event.status not in (MonitorTopicStatus.ACTIVE.value,)
            )
            or (
                previous_status == MonitorTopicStatus.ACTIVE.value
                and event.status
                not in (
                    MonitorTopicStatus.PAUSED.value,
                    MonitorTopicStatus.ARCHIVED.value,
                )
            )
            or previous_status == MonitorTopicStatus.ARCHIVED.value
        ):
            return None
        previous_status = event.status
        previous_at = event.occurred_at
    version = versions[topic_rule_version - 1]
    return TopicAnalysisRuleTimeline(
        rules=normalize_monitor_rules(
            match_any=version.match_any,
            match_all=version.match_all,
            exclude=version.exclude,
        ),
        source_keys=tuple(version.source_keys),
        starts_at=version.created_at,
        ends_at=(
            versions[topic_rule_version].created_at if topic_rule_version < len(versions) else None
        ),
        active_spans=tuple(
            (event.occurred_at, events[index + 1].occurred_at if index + 1 < len(events) else None)
            for index, event in enumerate(events)
            if event.status == MonitorTopicStatus.ACTIVE.value
        ),
    )


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


def _is_ascii_word_char(value: str) -> bool:
    return value.isascii() and value.isalnum()


def _contains_keyword(comparable_content: str, keyword: str) -> bool:
    """Match CJK keywords as substrings, but Latin edges only at word boundaries.

    Without boundaries a short keyword such as "AI" matches inside "daily" or "email".
    """
    key = _comparison_key(keyword)
    start = comparable_content.find(key)
    while start != -1:
        end = start + len(key)
        before_ok = not (
            _is_ascii_word_char(key[0])
            and start > 0
            and _is_ascii_word_char(comparable_content[start - 1])
        )
        after_ok = not (
            _is_ascii_word_char(key[-1])
            and end < len(comparable_content)
            and _is_ascii_word_char(comparable_content[end])
        )
        if before_ok and after_ok:
            return True
        start = comparable_content.find(key, start + 1)
    return False


def evaluate_monitor_rules(rules: NormalizedMonitorRules, content: str) -> MonitorRuleMatch:
    comparable_content = _comparison_key(_normalize_text(content))
    matched_any = tuple(
        keyword for keyword in rules.match_any if _contains_keyword(comparable_content, keyword)
    )
    matched_all = tuple(
        keyword for keyword in rules.match_all if _contains_keyword(comparable_content, keyword)
    )
    excluded_by = tuple(
        keyword for keyword in rules.exclude if _contains_keyword(comparable_content, keyword)
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
        owner_id: UUID | None = None,
        source_key: str | None = None,
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
                true() if owner_id is None else MonitorSchedule.owner_id == owner_id,
                true() if source_key is None else MonitorSchedule.source_key == source_key,
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
        job_id: UUID | None,
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
        if job_id is not None:
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

    def _record_status_event(
        self, *, topic: MonitorTopic, reason: str, occurred_at: datetime
    ) -> None:
        last_sequence = self._session.scalar(
            select(MonitorTopicStatusEvent.event_sequence)
            .where(
                MonitorTopicStatusEvent.owner_id == topic.owner_id,
                MonitorTopicStatusEvent.topic_id == topic.id,
            )
            .order_by(MonitorTopicStatusEvent.event_sequence.desc())
            .limit(1)
        )
        self._session.add(
            MonitorTopicStatusEvent(
                id=uuid4(),
                owner_id=topic.owner_id,
                topic_id=topic.id,
                event_sequence=(last_sequence or 0) + 1,
                topic_rule_version=topic.current_version,
                status=topic.status,
                reason=reason,
                occurred_at=occurred_at,
            )
        )

    def create_topic(
        self,
        *,
        owner_id: UUID,
        command: MonitorTopicCreateInput,
    ) -> MonitorTopicView:
        from connections.editorial_topic_sources import (
            require_editorial_topic_profiles_in_transaction,
        )

        name = self._normalize_name(command.name)
        rules = self._normalize_command_rules(command)
        source_keys = self._normalize_source_keys(command.source_keys)
        profile_ids = tuple(sorted(set(command.editorial_profile_ids), key=str))
        notification_target_names = self._normalize_notification_targets(
            command.notification_target_names
        )
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            require_editorial_topic_profiles_in_transaction(
                self._session, owner_id=owner_id, profile_ids=profile_ids, now=now
            )
            source_intervals = self._require_applied_search_sources(
                owner_id=owner_id, source_keys=source_keys
            )
            topic = MonitorTopic(
                id=uuid4(),
                owner_id=owner_id,
                name=name,
                status=MonitorTopicStatus.PAUSED.value,
                readiness_status=(
                    MonitorTopicReadinessStatus.READY.value
                    if source_keys or profile_ids
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
                source_keys=list(source_keys),
                editorial_profile_ids=[str(value) for value in profile_ids],
                collection_interval_seconds=command.collection_interval_seconds,
                created_at=now,
            )
            self._session.add(topic)
            self._session.flush()
            self._session.add(version)
            self._record_status_event(topic=topic, reason="created", occurred_at=now)
            self._sync_search_schedules(
                topic=topic, source_keys=source_keys, source_intervals=source_intervals, now=now
            )
            view = self._view(topic, version, source_keys=source_keys)
        return view

    def list_editorial_sources(self, *, owner_id: UUID) -> list[EditorialTopicSourceView]:
        from connections.editorial_topic_sources import list_editorial_topic_sources_in_transaction

        self._session.rollback()
        with self._session.begin():
            return [
                EditorialTopicSourceView(
                    profile_id=item.profile_id,
                    source_key=item.source_key,
                    name=item.name,
                    configuration_version=item.configuration_version,
                    enabled=item.enabled,
                    query_mode=item.query_mode,
                    text_scope=item.text_scope,
                    selectable=item.selectable,
                    reason=item.reason,
                    last_ok_at=item.last_ok_at,
                    interval_minutes=item.interval_minutes,
                )
                for item in list_editorial_topic_sources_in_transaction(
                    self._session, owner_id=owner_id, now=self._clock()
                )
            ]

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
        reject_internal_editorial_topic(owner_id=owner_id, topic_id=topic_id)
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

    def get_current_topic_rules_and_sources_in_transaction(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
    ) -> tuple[int, NormalizedMonitorRules, tuple[str, ...]]:
        """Expose the current rule and selected search sources without monitor ORM models."""
        if not self._session.in_transaction():
            raise RuntimeError("topic rules require the caller's transaction")
        topic = self._find_topic(owner_id=owner_id, topic_id=topic_id)
        version = self._find_version(topic)
        return (
            topic.current_version,
            normalize_monitor_rules(
                match_any=version.match_any,
                match_all=version.match_all,
                exclude=version.exclude,
            ),
            tuple(version.source_keys),
        )

    def lock_topic_for_event_commit_in_transaction(self, *, owner_id: UUID, topic_id: UUID) -> None:
        """Serialize automatic event assignment with manual topic operations."""
        if not self._session.in_transaction():
            raise RuntimeError("event topic lock requires the caller's transaction")
        found = self._session.scalar(
            select(MonitorTopic.id)
            .where(MonitorTopic.owner_id == owner_id, MonitorTopic.id == topic_id)
            .with_for_update()
        )
        if found is None:
            raise ValueError("event topic is absent")

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
                .where(
                    MonitorTopic.owner_id == owner_id,
                    MonitorTopic.id != editorial_event_topic_id(owner_id),
                )
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
        from connections.editorial_topic_sources import (
            require_editorial_topic_profiles_in_transaction,
        )

        reject_internal_editorial_topic(owner_id=owner_id, topic_id=topic_id)
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            source = self._find_topic(owner_id=owner_id, topic_id=topic_id)
            source_version = self._find_version(source)
            source_keys = self._source_keys(source)
            profile_ids = tuple(UUID(value) for value in source_version.editorial_profile_ids)
            require_editorial_topic_profiles_in_transaction(
                self._session, owner_id=owner_id, profile_ids=profile_ids, now=now
            )
            source_intervals = self._require_applied_search_sources(
                owner_id=owner_id, source_keys=source_keys
            )
            clone = MonitorTopic(
                id=uuid4(),
                owner_id=owner_id,
                name=source.name,
                status=MonitorTopicStatus.PAUSED.value,
                readiness_status=(
                    MonitorTopicReadinessStatus.READY.value
                    if source_keys or profile_ids
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
                source_keys=list(source_keys),
                editorial_profile_ids=[str(value) for value in profile_ids],
                collection_interval_seconds=source.collection_interval_seconds,
                created_at=now,
            )
            self._session.add(clone)
            self._session.flush()
            self._session.add(clone_version)
            self._record_status_event(topic=clone, reason="cloned", occurred_at=now)
            self._sync_search_schedules(
                topic=clone, source_keys=source_keys, source_intervals=source_intervals, now=now
            )
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
        from connections.editorial_topic_sources import (
            require_editorial_topic_profiles_in_transaction,
        )

        reject_internal_editorial_topic(owner_id=owner_id, topic_id=topic_id)
        name = self._normalize_name(command.name)
        rules = self._normalize_command_rules(command)
        source_keys = self._normalize_source_keys(command.source_keys)
        profile_ids = tuple(sorted(set(command.editorial_profile_ids), key=str))
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
            source_intervals = self._require_applied_search_sources(
                owner_id=owner_id, source_keys=source_keys
            )
            require_editorial_topic_profiles_in_transaction(
                self._session, owner_id=owner_id, profile_ids=profile_ids, now=now
            )
            current = self._find_version(topic)
            rules_changed = (
                tuple(current.match_any) != rules.match_any
                or tuple(current.match_all) != rules.match_all
                or tuple(current.exclude) != rules.exclude
            )
            name_changed = topic.name != name
            collection_changed = (
                tuple(current.source_keys) != source_keys
                or tuple(current.editorial_profile_ids)
                != tuple(str(value) for value in profile_ids)
                or current.collection_interval_seconds != command.collection_interval_seconds
            )
            if (
                topic.status == MonitorTopicStatus.ACTIVE.value
                and collection_changed
                and source_keys
            ):
                self._require_available_source_budget(owner_id=owner_id, source_keys=source_keys)
            settings_changed = (
                collection_changed
                or topic.report_time != command.report_time
                or topic.weekly_report_enabled != command.weekly_report_enabled
                or tuple(topic.notification_target_names) != notification_target_names
            )
            if rules_changed or collection_changed:
                next_version = topic.current_version + 1
                current = MonitorTopicVersion(
                    topic_id=topic.id,
                    version=next_version,
                    created_by=owner_id,
                    match_any=list(rules.match_any),
                    match_all=list(rules.match_all),
                    exclude=list(rules.exclude),
                    source_keys=list(source_keys),
                    editorial_profile_ids=[str(value) for value in profile_ids],
                    collection_interval_seconds=command.collection_interval_seconds,
                    created_at=now,
                )
                self._session.add(current)
                topic.current_version = next_version
            if name_changed or rules_changed or settings_changed:
                topic.name = name
                if (
                    not (source_keys or profile_ids)
                    and topic.status == MonitorTopicStatus.ACTIVE.value
                ):
                    topic.status = MonitorTopicStatus.PAUSED.value
                    self._record_status_event(
                        topic=topic, reason="source_selection", occurred_at=now
                    )
                topic.readiness_status = (
                    MonitorTopicReadinessStatus.READY.value
                    if source_keys or profile_ids
                    else MonitorTopicReadinessStatus.PENDING_SOURCE_SELECTION.value
                )
                topic.collection_interval_seconds = command.collection_interval_seconds
                topic.report_time = command.report_time
                topic.weekly_report_enabled = command.weekly_report_enabled
                topic.notification_target_names = list(notification_target_names)
                topic.updated_at = now
            self._sync_search_schedules(
                topic=topic,
                source_keys=source_keys,
                source_intervals=source_intervals,
                now=now,
            )
            view = self._view(topic, current, source_keys=source_keys)
        return view

    def _transition_topic(
        self,
        *,
        owner_id: UUID,
        topic_id: UUID,
        target: MonitorTopicStatus,
    ) -> MonitorTopicView:
        from connections.editorial_topic_sources import (
            require_editorial_topic_profiles_in_transaction,
        )

        reject_internal_editorial_topic(owner_id=owner_id, topic_id=topic_id)
        now = self._clock()
        self._session.rollback()
        with self._session.begin():
            topic = self._find_topic(
                owner_id=owner_id,
                topic_id=topic_id,
                for_update=True,
            )
            previous_status = topic.status
            if topic.status == MonitorTopicStatus.ARCHIVED.value:
                if target != MonitorTopicStatus.ARCHIVED:
                    raise ApplicationError("topic_archived")
            elif target == MonitorTopicStatus.ACTIVE:
                source_keys = self._source_keys(topic)
                profile_ids = tuple(
                    UUID(value) for value in self._find_version(topic).editorial_profile_ids
                )
                if not (source_keys or profile_ids):
                    raise ApplicationError("topic_not_ready")
                require_editorial_topic_profiles_in_transaction(
                    self._session, owner_id=owner_id, profile_ids=profile_ids, now=now
                )
                source_intervals = self._require_applied_search_sources(
                    owner_id=owner_id, source_keys=source_keys
                )
                self._require_available_source_budget(owner_id=owner_id, source_keys=source_keys)
                topic.readiness_status = MonitorTopicReadinessStatus.READY.value
                if topic.status != target.value:
                    topic.status = target.value
                    topic.updated_at = now
                self._set_schedules_enabled(topic=topic, enabled=True, now=now)
                self._sync_search_schedules(
                    topic=topic,
                    source_keys=source_keys,
                    source_intervals=source_intervals,
                    now=now,
                )
            elif topic.status != target.value:
                topic.status = target.value
                topic.updated_at = now
            if target in {MonitorTopicStatus.PAUSED, MonitorTopicStatus.ARCHIVED}:
                self._set_schedules_enabled(topic=topic, enabled=False, now=now)
            if topic.status != previous_status:
                reason = {
                    MonitorTopicStatus.ACTIVE: "resumed",
                    MonitorTopicStatus.PAUSED: "paused",
                    MonitorTopicStatus.ARCHIVED: "archived",
                }[target]
                self._record_status_event(topic=topic, reason=reason, occurred_at=now)
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
    ) -> dict[str, int]:
        applied = load_applied_source_presets_in_transaction(
            self._session,
            owner_id=owner_id,
            source_keys=source_keys,
        )
        if set(applied) != set(source_keys) or any(
            SourceCapability.SEARCH not in preset.capabilities for preset in applied.values()
        ):
            raise ApplicationError("source_preset_not_applied")
        readiness = load_source_access_readiness(
            self._session, owner_id=owner_id, now=self._clock()
        )
        intervals: dict[str, int] = {}
        for source_key, preset in applied.items():
            policy = load_execution_policy_in_transaction(
                self._session,
                owner_id=owner_id,
                connection_id=preset.connection_id,
                connection_version=preset.connection_version,
            )
            if (
                not readiness.get((source_key, SourceCapability.SEARCH), False)
                or not policy.enabled
            ):
                raise ApplicationError("source_preset_not_applied")
            intervals[source_key] = policy.min_interval_seconds
        return intervals

    def _require_available_source_budget(
        self, *, owner_id: UUID, source_keys: tuple[str, ...]
    ) -> None:
        snapshots = ResourceBudgetService(self._session, clock=self._clock).budget_usage_snapshot(
            owner_id=owner_id
        )
        for source_key in source_keys:
            matching = [
                budget
                for budget in snapshots
                if budget.metric is BudgetMetric.NETWORK_REQUEST
                and (
                    (
                        budget.scope_kind is BudgetScopeKind.SOURCE
                        and budget.scope_reference == source_key
                    )
                    or budget.scope_kind is BudgetScopeKind.GLOBAL
                )
            ]
            if not {BudgetScopeKind.GLOBAL, BudgetScopeKind.SOURCE}.issubset(
                {budget.scope_kind for budget in matching}
            ) or any(
                not budget.enabled or budget.remaining_units is None or budget.remaining_units < 1
                for budget in matching
            ):
                raise ApplicationError("topic_not_ready")

    def _sync_search_schedules(
        self,
        *,
        topic: MonitorTopic,
        source_keys: tuple[str, ...],
        source_intervals: dict[str, int],
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
                        interval_seconds=max(
                            topic.collection_interval_seconds, source_intervals[source_key]
                        ),
                        next_run_at=now,
                        enabled=topic.status == MonitorTopicStatus.ACTIVE.value,
                        last_job_id=None,
                        created_at=now,
                        updated_at=now,
                    )
                )
                continue
            interval_seconds = max(topic.collection_interval_seconds, source_intervals[source_key])
            enabled = topic.status == MonitorTopicStatus.ACTIVE.value
            # A shortened cadence must not retain yesterday's longer future deadline.
            # Keep overdue work due, and never bypass the source's minimum interval.
            next_run_at = min(
                existing_schedule.next_run_at, now + timedelta(seconds=interval_seconds)
            )
            if (
                existing_schedule.interval_seconds != interval_seconds
                or existing_schedule.enabled != enabled
                or existing_schedule.next_run_at != next_run_at
            ):
                existing_schedule.next_run_at = next_run_at
                existing_schedule.interval_seconds = interval_seconds
                existing_schedule.enabled = enabled
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
            editorial_profile_ids=[UUID(value) for value in version.editorial_profile_ids],
            collection_interval_seconds=topic.collection_interval_seconds,
            report_time=topic.report_time,
            report_timezone="Asia/Shanghai",
            weekly_report_enabled=topic.weekly_report_enabled,
            notification_target_names=list(topic.notification_target_names),
            created_at=topic.created_at.astimezone(UTC),
            updated_at=topic.updated_at.astimezone(UTC),
        )
