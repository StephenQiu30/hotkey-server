from __future__ import annotations

import os
from collections.abc import Iterator
from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from sqlalchemy import create_engine, text
from sqlalchemy.engine import make_url
from sqlalchemy.orm import Session

from identity.models import IdentityUser
from monitors.schemas import FollowedAccountIdentityInput
from monitors.services import FollowedAccountService


@pytest.fixture
def followed_account_session() -> Iterator[tuple[Session, IdentityUser]]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")
    if make_url(database_url).database != "hotkey_test":
        pytest.fail("followed account integration tests require the isolated hotkey_test database")

    engine = create_engine(database_url)
    now = datetime(2026, 9, 25, tzinfo=UTC)
    owner = IdentityUser(
        id=uuid4(),
        singleton_key=1,
        username="followed-account-owner",
        password_hash="test-only-hash",
        credential_version=1,
        created_at=now,
        updated_at=now,
    )
    with engine.begin() as connection:
        connection.execute(text("DELETE FROM identity_users WHERE singleton_key = 1"))
        connection.execute(
            text(
                "INSERT INTO identity_users "
                "(id, singleton_key, username, password_hash, credential_version, "
                "created_at, updated_at) "
                "VALUES (:id, 1, :username, :password_hash, 1, :created_at, :updated_at)"
            ),
            {
                "id": owner.id,
                "username": owner.username,
                "password_hash": owner.password_hash,
                "created_at": now,
                "updated_at": now,
            },
        )

    session = Session(engine)
    try:
        yield session, owner
    finally:
        session.close()
        with engine.begin() as connection:
            connection.execute(
                text("DELETE FROM identity_users WHERE id = :owner_id"),
                {"owner_id": owner.id},
            )
        engine.dispose()


def test_postgres_keeps_stable_identity_and_owner_scoped_alias_history(
    followed_account_session: tuple[Session, IdentityUser],
) -> None:
    session, owner = followed_account_session
    observed_at = datetime(2026, 9, 25, tzinfo=UTC)
    service = FollowedAccountService(session, clock=lambda: observed_at)

    first = service.record_confirmed_identity(
        owner_id=owner.id,
        identity=FollowedAccountIdentityInput(
            source_key="x",
            external_id="1001",
            alias_value="small",
            display_name="相同展示名",
        ),
    )
    observed_at += timedelta(hours=1)
    renamed = service.record_confirmed_identity(
        owner_id=owner.id,
        identity=FollowedAccountIdentityInput(
            source_key="x",
            external_id="1001",
            alias_value="small_new",
            display_name="新展示名",
        ),
    )
    reused = service.record_confirmed_identity(
        owner_id=owner.id,
        identity=FollowedAccountIdentityInput(
            source_key="x",
            external_id="2002",
            alias_value="small",
            display_name="相同展示名",
        ),
    )

    assert renamed.id == first.id
    assert renamed.external_id == "1001"
    assert renamed.latest_observed_alias == "small_new"
    assert {
        candidate.external_id
        for candidate in service.find_by_alias(
            owner_id=owner.id,
            source_key="x",
            alias_value="small",
        )
    } == {"1001", "2002"}
    assert reused.id != first.id
    assert (
        service.find_by_alias(
            owner_id=uuid4(),
            source_key="x",
            alias_value="small",
        )
        == []
    )
