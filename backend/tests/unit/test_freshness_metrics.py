from datetime import UTC, datetime, timedelta, timezone

import pytest
from pydantic import ValidationError

from jobs.schemas import (
    FreshnessTimelineInput,
    SourceTimePrecision,
    SourceTimeStatus,
)
from jobs.services import measure_freshness_timeline

BASE = datetime(2026, 9, 21, 12, tzinfo=UTC)


def _timeline(**changes: object) -> FreshnessTimelineInput:
    values: dict[str, object] = {
        "scheduled_for_at": BASE,
        "accepted_at": BASE + timedelta(seconds=2),
        "started_at": BASE + timedelta(seconds=5),
        "request_started_at": BASE + timedelta(seconds=7),
        "source_published_at": BASE - timedelta(minutes=10),
        "source_time_precision": SourceTimePrecision.SECOND,
        "source_observed_at": BASE + timedelta(seconds=11),
        "persisted_at": BASE + timedelta(seconds=14),
        "queryable_at": BASE + timedelta(seconds=15),
    }
    values.update(changes)
    return FreshnessTimelineInput(**values)


def test_complete_timeline_separates_each_non_negative_duration() -> None:
    source_time = datetime(2026, 9, 21, 19, 50, tzinfo=timezone(timedelta(hours=8)))

    measured = measure_freshness_timeline(_timeline(source_published_at=source_time))

    assert measured.source_time_status is SourceTimeStatus.VALID
    assert measured.schedule_wait_us == 5_000_000
    assert measured.queue_wait_us == 3_000_000
    assert measured.internal_prepare_us == 2_000_000
    assert measured.source_wait_us == 4_000_000
    assert measured.processing_us == 3_000_000
    assert measured.visibility_us == 1_000_000
    assert measured.end_to_end_us == 15_000_000
    assert measured.publication_to_observation_us == 611_000_000


def test_missing_stages_remain_unknown_instead_of_becoming_zero() -> None:
    measured = measure_freshness_timeline(
        _timeline(
            scheduled_for_at=None,
            started_at=None,
            request_started_at=None,
            source_published_at=None,
            source_time_precision=SourceTimePrecision.UNKNOWN,
            source_observed_at=None,
            persisted_at=None,
            queryable_at=None,
        )
    )

    assert measured.source_time_status is SourceTimeStatus.UNKNOWN
    assert measured.schedule_wait_us is None
    assert measured.queue_wait_us is None
    assert measured.source_wait_us is None
    assert measured.publication_to_observation_us is None


def test_source_time_anomalies_never_produce_negative_discovery_delay() -> None:
    missing_timezone = measure_freshness_timeline(
        _timeline(source_published_at=datetime(2026, 9, 21, 12))
    )
    future = measure_freshness_timeline(_timeline(source_published_at=BASE + timedelta(minutes=1)))

    assert missing_timezone.source_time_status is SourceTimeStatus.MISSING_TIMEZONE
    assert missing_timezone.publication_to_observation_us is None
    assert future.source_time_status is SourceTimeStatus.FUTURE_SKEW
    assert future.publication_to_observation_us is None


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("accepted_at", datetime(2026, 9, 21, 12)),
        ("started_at", BASE + timedelta(seconds=1)),
        ("request_started_at", None),
        ("persisted_at", BASE + timedelta(seconds=10)),
        ("queryable_at", BASE + timedelta(seconds=13)),
    ],
)
def test_invalid_internal_time_chains_are_rejected(field: str, value: object) -> None:
    with pytest.raises(ValidationError):
        _timeline(**{field: value})
