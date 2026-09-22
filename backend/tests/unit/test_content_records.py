from __future__ import annotations

from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from pydantic import ValidationError

from connections.schemas import SourceEntryPoint
from content.schemas import (
    ContentMetricView,
    ContentVisibilityBasis,
    ContentVisibilityStatus,
    PersistContentPostInput,
    RecordContentVisibilityInput,
)
from evidence.schemas import AdmittedSourcePayload, DataClass
from sources.contracts import SourceCapability


def _admission(*, fields: dict[str, object]) -> AdmittedSourcePayload:
    collected_at = datetime(2026, 9, 22, 8, 0, tzinfo=UTC)
    return AdmittedSourcePayload(
        policy_id=uuid4(),
        policy_version=1,
        owner_id=uuid4(),
        source_key="x",
        capability=SourceCapability.SEARCH,
        retention_policy_id=uuid4(),
        retention_policy_version=1,
        data_class=DataClass.STRUCTURED,
        collected_at=collected_at,
        expires_at=collected_at + timedelta(days=30),
        fields=fields,
    )


def _command(*, fields: dict[str, object], native_scope: str | None = None) -> dict[str, object]:
    return {
        "job_id": uuid4(),
        "source_operation_id": uuid4(),
        "connection_id": uuid4(),
        "entry_point": SourceEntryPoint.MANUAL,
        "component_name": "controlled-collector",
        "component_version": "1",
        "native_scope": native_scope,
        "admission": _admission(fields=fields),
    }


def test_metric_view_keeps_known_zero_separate_from_unknown() -> None:
    metrics = ContentMetricView(
        like_count=0,
        comment_count=None,
        repost_count=None,
        view_count=None,
        play_count=0,
        danmaku_count=None,
    )

    assert metrics.model_dump() == {
        "like_count": 0,
        "comment_count": None,
        "repost_count": None,
        "view_count": None,
        "play_count": 0,
        "danmaku_count": None,
    }


def test_persist_input_requires_stable_source_identity() -> None:
    with pytest.raises(ValidationError, match="external_id"):
        PersistContentPostInput.model_validate(
            _command(
                fields={
                    "canonical_url": "https://example.invalid/posts/1",
                    "like_count": 0,
                }
            )
        )


@pytest.mark.parametrize("native_scope", ["", " account-1", "account-1\n"])
def test_persist_input_rejects_empty_or_rewritten_native_scope(native_scope: str) -> None:
    with pytest.raises(ValidationError, match="native_scope"):
        PersistContentPostInput.model_validate(
            _command(fields={"external_id": "post-001"}, native_scope=native_scope)
        )


def test_persist_input_rejects_naive_source_times() -> None:
    admission = _admission(fields={"external_id": "post-001"}).model_copy(
        update={"collected_at": datetime(2026, 9, 22, 8, 0)}
    )

    with pytest.raises(ValidationError, match="collected_at"):
        PersistContentPostInput.model_validate(
            {**_command(fields={"external_id": "post-001"}), "admission": admission}
        )


@pytest.mark.parametrize(
    ("status", "basis"),
    [
        (ContentVisibilityStatus.VISIBLE, ContentVisibilityBasis.NOT_FOUND),
        (ContentVisibilityStatus.DELETED, ContentVisibilityBasis.TIMEOUT),
        (ContentVisibilityStatus.RESTRICTED, ContentVisibilityBasis.HTTP_GONE),
        (ContentVisibilityStatus.TRANSIENT_FAILURE, ContentVisibilityBasis.ACCESS_DENIED),
        (ContentVisibilityStatus.UNKNOWN, ContentVisibilityBasis.SOURCE_TOMBSTONE),
    ],
)
def test_visibility_input_rejects_status_without_matching_evidence(
    status: ContentVisibilityStatus,
    basis: ContentVisibilityBasis,
) -> None:
    with pytest.raises(ValidationError, match="basis"):
        RecordContentVisibilityInput(
            content_id=uuid4(),
            job_id=uuid4(),
            source_operation_id=uuid4(),
            observed_at=datetime(2026, 9, 22, 8, 0, tzinfo=UTC),
            status=status,
            basis=basis,
        )


def test_visibility_input_requires_source_time_with_timezone() -> None:
    with pytest.raises(ValidationError, match="observed_at"):
        RecordContentVisibilityInput(
            content_id=uuid4(),
            job_id=uuid4(),
            source_operation_id=uuid4(),
            observed_at=datetime(2026, 9, 22, 8, 0),
            status=ContentVisibilityStatus.TRANSIENT_FAILURE,
            basis=ContentVisibilityBasis.TIMEOUT,
        )
