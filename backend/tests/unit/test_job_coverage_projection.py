from __future__ import annotations

from datetime import UTC, datetime
from uuid import uuid4

from jobs.schemas import CoverageWindowStatus, CoverageWindowView, JobStatusView


def test_job_status_contract_exposes_persisted_coverage_windows() -> None:
    properties = JobStatusView.model_json_schema()["properties"]
    assert "coverage_windows" in properties


def test_coverage_window_view_includes_utc_range() -> None:
    starts_at = datetime(2026, 9, 21, tzinfo=UTC)
    ends_at = datetime(2026, 9, 22, tzinfo=UTC)
    view = CoverageWindowView(
        id=uuid4(),
        status=CoverageWindowStatus.PARTIAL,
        stop_reason="cursor_loop",
        page_count=2,
        starts_at=starts_at,
        ends_at=ends_at,
    )

    assert view.starts_at == starts_at
    assert view.ends_at == ends_at
