from __future__ import annotations

import os
from collections.abc import Iterator, Mapping
from datetime import UTC, datetime, timedelta
from typing import Any
from uuid import UUID, uuid4

import pytest
from sqlalchemy import Engine, create_engine, text
from sqlalchemy.orm import sessionmaker

from ai.schemas import AiCallError, AiCompletion, AiFailureCode, AiTokenUsage
from ai.services import AiService
from jobs.schemas import (
    BudgetMetric,
    BudgetPolicyInput,
    BudgetScopeKind,
    ComponentPolicyInput,
    CostClass,
)
from jobs.services import ResourceBudgetService

_NOW = datetime(2026, 9, 25, 9, tzinfo=UTC)


class _FakeClient:
    provider = "fake"
    model = "fake-model"

    def __init__(self, failure: AiFailureCode | None = None) -> None:
        self.failure = failure
        self.prompts: list[str] = []

    def complete(
        self, *, prompt: str, output_schema: Mapping[str, Any], instructions: str = ""
    ) -> AiCompletion:
        self.prompts.append(prompt)
        if self.failure is not None:
            raise AiCallError(self.failure, "secret prompt text must not be stored")
        return AiCompletion(
            provider=self.provider,
            model=self.model,
            output={"sentiment": "neutral"},
            usage=AiTokenUsage(input_tokens=100, output_tokens=5),
            duration_ms=42,
        )


@pytest.fixture
def engine() -> Iterator[Engine]:
    database_url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if database_url is None:
        pytest.skip("HOTKEY_TEST_DATABASE_URL is required for PostgreSQL integration tests")
    engine = create_engine(database_url)
    with engine.begin() as connection:
        connection.execute(
            text("TRUNCATE knowledge_exports, ai_calls, identity_sessions, identity_users CASCADE")
        )
    try:
        yield engine
    finally:
        with engine.begin() as connection:
            connection.execute(
                text(
                    "TRUNCATE knowledge_exports, "
                    "ai_calls, "
                    "identity_sessions, identity_users CASCADE"
                )
            )
        engine.dispose()


def _owner(engine: Engine) -> UUID:
    owner_id = uuid4()
    with engine.begin() as connection:
        connection.execute(
            text(
                "INSERT INTO identity_users (id, username, password_hash, created_at, updated_at) "
                "VALUES (:id, 'ai-owner', 'x', :now, :now)"
            ),
            {"id": owner_id, "now": _NOW},
        )
    return owner_id


def _enable_ai_budget(engine: Engine, owner_id: UUID) -> None:
    sessions = sessionmaker(engine)
    with sessions() as session:
        service = ResourceBudgetService(session, clock=lambda: _NOW)
        service.save_component_policy(
            owner_id=owner_id,
            command=ComponentPolicyInput(
                component_key="codex.app-server",
                component_version="0.146.0",
                cost_class=CostClass.LOCAL,
                enabled_for_core=True,
                terms_reference="local-codex-app-server",
                reviewed_at=_NOW,
            ),
        )
        service.save_budget_policy(
            owner_id=owner_id,
            command=BudgetPolicyInput(
                budget_key="global.analysis-attempts",
                metric=BudgetMetric.ANALYSIS_ATTEMPT,
                scope_kind=BudgetScopeKind.GLOBAL,
                limit_units=10,
                window_seconds=3600,
                window_anchor_at=_NOW - timedelta(minutes=1),
                enabled=True,
            ),
        )


def test_ai_calls_record_success_and_failure_without_prompt_text(engine: Engine) -> None:
    owner_id = _owner(engine)
    _enable_ai_budget(engine, owner_id)
    sessions = sessionmaker(engine)
    ok_client = _FakeClient()
    with sessions() as session:
        result = AiService(session, ok_client, clock=lambda: _NOW).complete(
            owner_id=owner_id,
            job_id=None,
            purpose="analysis.annotate",
            prompt_version="annotate-v1",
            prompt="<data>内容</data>",
            output_schema={"type": "object"},
        )
    assert result.output == {"sentiment": "neutral"}

    with sessions() as session, pytest.raises(AiCallError):
        AiService(session, _FakeClient(AiFailureCode.RATE_LIMITED), clock=lambda: _NOW).complete(
            owner_id=owner_id,
            job_id=None,
            purpose="analysis.annotate",
            prompt_version="annotate-v1",
            prompt="<data>内容</data>",
            output_schema={"type": "object"},
        )

    with engine.connect() as connection:
        rows = connection.execute(
            text(
                "SELECT status, failure_code, provider, model, input_tokens, output_tokens, "
                "duration_ms, octet_length(input_fingerprint) FROM ai_calls "
                "WHERE owner_id = :owner_id ORDER BY status DESC"
            ),
            {"owner_id": owner_id},
        ).all()
        stored_text = connection.execute(text("SELECT ai_calls::text FROM ai_calls")).scalars()
        assert not any("内容" in row or "secret" in row for row in stored_text)
        usage_rows = connection.execute(
            text(
                "SELECT r.status, r.actual_units, a.outcome "
                "FROM resource_budget_reservations r "
                "JOIN resource_usage_attempts a "
                "ON a.owner_id = r.owner_id AND a.attempt_id = r.reservation_id "
                "WHERE r.owner_id = :owner_id ORDER BY a.outcome DESC"
            ),
            {"owner_id": owner_id},
        ).all()
        budget_window = connection.execute(
            text(
                "SELECT used_units, reserved_units FROM resource_budget_windows "
                "WHERE owner_id = :owner_id"
            ),
            {"owner_id": owner_id},
        ).one()
    assert [tuple(row) for row in rows] == [
        ("succeeded", None, "fake", "fake-model", 100, 5, 42, 32),
        ("failed", "rate_limited", "fake", "fake-model", 0, 0, 0, 32),
    ]
    assert [tuple(row) for row in usage_rows] == [
        ("settled", 1, "succeeded"),
        ("settled", 1, "failed"),
    ]
    assert tuple(budget_window) == (2, 0)
