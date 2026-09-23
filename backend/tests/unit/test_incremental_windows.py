import hashlib
from datetime import UTC, datetime, timedelta
from uuid import uuid4

import pytest
from pydantic import ValidationError

from jobs.schemas import CoverageWindowInput
from sources.contracts import SourceCapability, SourceSort

_TARGET_HASH = hashlib.sha256(b"controlled-target").digest()


def _window(**changes: object) -> dict[str, object]:
    end = datetime(2026, 9, 23, 2, tzinfo=UTC)
    return {
        "owner_id": uuid4(),
        "source_key": "x",
        "capability": SourceCapability.SEARCH,
        "target_hash": _TARGET_HASH,
        "sort_key": SourceSort.LATEST,
        "rule_version": 1,
        "starts_at": end - timedelta(hours=1),
        "ends_at": end,
        **changes,
    }


@pytest.mark.parametrize(
    "changes",
    [
        {"starts_at": datetime(2026, 9, 23, 1)},
        {"starts_at": datetime(2026, 9, 23, 2, tzinfo=UTC)},
        {"target_hash": b"short"},
        {"rule_version": 0},
        {"capability": SourceCapability.PAGE_CONTENT},
    ],
)
def test_window_rejects_ambiguous_or_invalid_scope(changes: dict[str, object]) -> None:
    with pytest.raises(ValidationError):
        CoverageWindowInput.model_validate(_window(**changes))


def test_window_accepts_explicit_utc_half_open_range() -> None:
    window = CoverageWindowInput.model_validate(_window())

    assert window.starts_at < window.ends_at
    assert window.starts_at.utcoffset() == timedelta(0)
