from datetime import UTC, datetime, timedelta
from typing import cast
from uuid import uuid4

import pytest
from sqlalchemy.dialects import postgresql
from sqlalchemy.orm import Session

from jobs.models import Job
from jobs.schemas import JobDelayReason
from jobs.services import JobService

NOW = datetime(2026, 9, 24, 12, tzinfo=UTC)


def _job(
    *,
    status: str = "queued",
    source_key: str | None = "x",
    source_capability: str | None = "search",
    defer_reason: str | None = None,
    last_error_code: str | None = None,
    scheduled_for_at: datetime | None = None,
    updated_at: datetime = NOW - timedelta(minutes=3),
) -> Job:
    return Job(
        id=uuid4(),
        owner_id=uuid4(),
        operation_id=uuid4(),
        kind="monitor.collect",
        configuration_ref="monitor-1",
        configuration_version=2,
        source_key=source_key,
        source_capability=source_capability,
        scope={},
        request_fingerprint=b"x" * 32,
        status=status,
        scheduled_for_at=scheduled_for_at,
        defer_reason=defer_reason,
        last_error_code=last_error_code,
        created_at=NOW - timedelta(minutes=10),
        updated_at=updated_at,
    )


class _AggregateSession:
    def __init__(self, aggregate: tuple[datetime | None, datetime | None]) -> None:
        self.aggregate = aggregate
        self.statement = None

    def execute(self, statement):
        self.statement = statement
        return self

    def one(self) -> tuple[datetime | None, datetime | None]:
        return self.aggregate


def test_source_freshness_uses_attempt_and_complete_success_facts() -> None:
    last_attempt = NOW - timedelta(minutes=2)
    last_success = NOW - timedelta(hours=2)
    session = _AggregateSession((last_attempt, last_success))
    view = JobService(cast(Session, session))._source_freshness(_job(), now=NOW)

    assert view is not None
    assert view.last_attempt_at == last_attempt
    assert view.last_success_at == last_success
    assert view.delay_reason is JobDelayReason.INTERNAL_QUEUE


@pytest.mark.parametrize(
    ("defer_reason", "error_code", "expected"),
    [
        ("rate_limited", "http_429", JobDelayReason.RATE_LIMITED),
        ("rate_limited", "collector_budget_exhausted", JobDelayReason.BUDGET_EXHAUSTED),
        ("transient", "upstream_timeout", JobDelayReason.TRANSIENT_FAILURE),
        ("manual_retry", "upstream_error", JobDelayReason.MANUAL_RETRY),
        ("provider_private_reason", "internal_error", JobDelayReason.OTHER),
    ],
)
def test_deferred_job_maps_reason_and_measures_nonnegative_wait(
    defer_reason: str,
    error_code: str,
    expected: JobDelayReason,
) -> None:
    since = NOW - timedelta(seconds=90)
    session = _AggregateSession((NOW - timedelta(minutes=2), NOW - timedelta(hours=1)))
    view = JobService(cast(Session, session))._source_freshness(
        _job(defer_reason=defer_reason, last_error_code=error_code, updated_at=since),
        now=NOW,
    )

    assert view is not None
    assert view.delay_reason is expected
    assert view.delay_since_at == since
    assert view.delay_duration_us == 90_000_000


def test_future_scheduled_job_is_not_reported_as_delayed() -> None:
    session = _AggregateSession((None, None))
    view = JobService(cast(Session, session))._source_freshness(
        _job(scheduled_for_at=NOW + timedelta(minutes=1)),
        now=NOW,
    )

    assert view is not None
    assert view.delay_reason is None
    assert view.delay_since_at is None
    assert view.delay_duration_us is None


def test_clock_skew_does_not_create_a_negative_delay() -> None:
    session = _AggregateSession((None, None))
    view = JobService(cast(Session, session))._source_freshness(
        _job(defer_reason="manual_retry", updated_at=NOW + timedelta(seconds=1)),
        now=NOW,
    )

    assert view is not None
    assert view.delay_reason is JobDelayReason.MANUAL_RETRY
    assert view.delay_duration_us is None


def test_non_source_job_does_not_query_or_return_source_freshness() -> None:
    session = _AggregateSession((None, None))
    view = JobService(cast(Session, session))._source_freshness(
        _job(source_key=None, source_capability=None),
        now=NOW,
    )

    assert view is None
    assert session.statement is None


def test_freshness_aggregate_is_owner_and_configuration_version_scoped() -> None:
    session = _AggregateSession((None, None))
    JobService(cast(Session, session))._source_freshness(_job(), now=NOW)
    assert session.statement is not None

    compiled = session.statement.compile(
        dialect=postgresql.dialect(),
        compile_kwargs={"literal_binds": True},
    )
    sql = str(compiled)

    assert "jobs.owner_id =" in sql
    assert "jobs.source_key = 'x'" in sql
    assert "jobs.source_capability = 'search'" in sql
    assert "jobs.configuration_ref = 'monitor-1'" in sql
    assert "jobs.configuration_version = 2" in sql
    assert "jobs.status = 'succeeded'" in sql
    assert "max(job_attempts.started_at)" in sql
