from __future__ import annotations

from collections.abc import Iterator
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest
from pydantic import ValidationError
from sqlalchemy import create_engine, event
from sqlalchemy.orm import Session
from sqlalchemy.pool import StaticPool

from core.errors import ApplicationError
from db.base import Base
from identity.models import IdentityUser
from monitors.models import FollowedAccount, FollowedAccountAlias
from monitors.schemas import FollowedAccountIdentityInput
from monitors.services import FollowedAccountService


@pytest.fixture
def account_session() -> Iterator[Session]:
    engine = create_engine(
        "sqlite://",
        connect_args={"check_same_thread": False},
        poolclass=StaticPool,
    )
    event.listen(
        engine,
        "connect",
        lambda connection, _: connection.execute("PRAGMA foreign_keys=ON"),
    )
    Base.metadata.create_all(
        engine,
        tables=[
            IdentityUser.__table__,
            FollowedAccount.__table__,
            FollowedAccountAlias.__table__,
        ],
    )
    session = Session(engine)
    now = datetime(2026, 9, 25, tzinfo=UTC)
    session.add(
        IdentityUser(
            id=uuid4(),
            singleton_key=1,
            username="account-owner",
            password_hash="test-only-hash",
            credential_version=1,
            created_at=now,
            updated_at=now,
        )
    )
    session.commit()
    try:
        yield session
    finally:
        session.close()
        engine.dispose()


def _owner_id(session: Session) -> UUID:
    user = session.query(IdentityUser).one()
    return user.id


def _without_timezone(value: datetime) -> datetime:
    return value.replace(tzinfo=None)


def _identity(
    *,
    external_id: str,
    alias_value: str,
    display_name: str = "相同展示名",
) -> FollowedAccountIdentityInput:
    return FollowedAccountIdentityInput(
        source_key="x",
        external_id=external_id,
        alias_value=alias_value,
        display_name=display_name,
    )


def test_stable_identity_survives_rename_and_keeps_alias_observations(
    account_session: Session,
) -> None:
    owner_id = _owner_id(account_session)
    observed_at = datetime(2026, 9, 25, tzinfo=UTC)
    service = FollowedAccountService(account_session, clock=lambda: observed_at)

    first = service.record_confirmed_identity(
        owner_id=owner_id,
        identity=_identity(external_id="1001", alias_value="small"),
    )
    observed_at += timedelta(hours=1)
    renamed = service.record_confirmed_identity(
        owner_id=owner_id,
        identity=_identity(
            external_id="1001",
            alias_value="small_new",
            display_name="新展示名",
        ),
    )

    assert renamed.id == first.id
    assert renamed.external_id == "1001"
    assert renamed.display_name == "新展示名"
    assert renamed.latest_observed_alias == "small_new"
    assert [
        (
            item.alias_value,
            _without_timezone(item.first_seen_at),
            _without_timezone(item.last_seen_at),
        )
        for item in renamed.aliases
    ] == [
        ("small", datetime(2026, 9, 25), datetime(2026, 9, 25)),
        ("small_new", datetime(2026, 9, 25, 1), datetime(2026, 9, 25, 1)),
    ]


def test_reused_alias_keeps_multiple_stable_identity_candidates(
    account_session: Session,
) -> None:
    owner_id = _owner_id(account_session)
    service = FollowedAccountService(account_session)

    first = service.record_confirmed_identity(
        owner_id=owner_id,
        identity=_identity(external_id="1001", alias_value="small"),
    )
    second = service.record_confirmed_identity(
        owner_id=owner_id,
        identity=_identity(external_id="2002", alias_value="small"),
    )
    candidates = service.find_by_alias(owner_id=owner_id, source_key="x", alias_value="small")

    assert first.id != second.id
    assert {candidate.external_id for candidate in candidates} == {"1001", "2002"}
    assert all(candidate.latest_observed_alias == "small" for candidate in candidates)


def test_reobserving_same_alias_updates_last_seen_without_duplicate_history(
    account_session: Session,
) -> None:
    owner_id = _owner_id(account_session)
    observed_at = datetime(2026, 9, 25, tzinfo=UTC)
    service = FollowedAccountService(account_session, clock=lambda: observed_at)

    service.record_confirmed_identity(
        owner_id=owner_id,
        identity=_identity(external_id="1001", alias_value="small"),
    )
    observed_at += timedelta(minutes=5)
    result = service.record_confirmed_identity(
        owner_id=owner_id,
        identity=_identity(external_id="1001", alias_value="small"),
    )

    assert len(result.aliases) == 1
    assert _without_timezone(result.aliases[0].first_seen_at) == datetime(2026, 9, 25)
    assert _without_timezone(result.aliases[0].last_seen_at) == datetime(2026, 9, 25, 0, 5)


def test_account_and_alias_observation_timestamps_do_not_regress_with_clock(
    account_session: Session,
) -> None:
    owner_id = _owner_id(account_session)
    observed_at = datetime(2026, 9, 25, tzinfo=UTC)
    service = FollowedAccountService(account_session, clock=lambda: observed_at)

    service.record_confirmed_identity(
        owner_id=owner_id,
        identity=_identity(external_id="1001", alias_value="small"),
    )
    observed_at += timedelta(minutes=5)
    latest = service.record_confirmed_identity(
        owner_id=owner_id,
        identity=_identity(external_id="1001", alias_value="small"),
    )

    observed_at -= timedelta(minutes=3)
    regressed = service.record_confirmed_identity(
        owner_id=owner_id,
        identity=_identity(external_id="1001", alias_value="small"),
    )

    assert regressed.id == latest.id
    assert _without_timezone(regressed.updated_at) == datetime(2026, 9, 25, 0, 5)
    assert _without_timezone(regressed.aliases[0].first_seen_at) == datetime(2026, 9, 25)
    assert _without_timezone(regressed.aliases[0].last_seen_at) == datetime(2026, 9, 25, 0, 5)


def test_account_read_hides_records_outside_owner_scope(account_session: Session) -> None:
    owner_id = _owner_id(account_session)
    account = FollowedAccountService(account_session).record_confirmed_identity(
        owner_id=owner_id,
        identity=_identity(external_id="1001", alias_value="small"),
    )
    service = FollowedAccountService(account_session)

    with pytest.raises(ApplicationError, match="resource_not_found"):
        service.get_account(owner_id=uuid4(), account_id=account.id)

    assert (
        service.find_by_alias(
            owner_id=uuid4(),
            source_key="x",
            alias_value="small",
        )
        == []
    )


@pytest.mark.parametrize(
    ("source_key", "external_id"),
    [("X", "1001"), ("x", "id with spaces"), ("x/unsafe", "1001")],
)
def test_identity_input_rejects_unstable_or_invalid_keys(
    source_key: str,
    external_id: str,
) -> None:
    with pytest.raises(ValidationError):
        FollowedAccountIdentityInput(
            source_key=source_key,
            external_id=external_id,
            alias_value="small",
            display_name="Name",
        )
