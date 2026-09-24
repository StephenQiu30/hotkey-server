from __future__ import annotations

import json
from datetime import UTC, datetime
from uuid import uuid4

from typer.testing import CliRunner

from cli import jobs as job_commands
from cli.commands import app
from jobs.schemas import JobReliabilityOutcome, JobReliabilitySnapshot


def _empty_snapshot(start: datetime, end: datetime) -> JobReliabilitySnapshot:
    return JobReliabilitySnapshot(
        window_start=start,
        window_end=end,
        sla_seconds=1800,
        total_jobs=0,
        on_time_jobs=0,
        outcome_counts={outcome: 0 for outcome in JobReliabilityOutcome},
        records=(),
    )


def test_job_reliability_cli_prints_read_only_snapshot_json(monkeypatch) -> None:
    owner_id = uuid4()
    start = datetime(2026, 9, 24, 12, tzinfo=UTC)
    end = datetime(2026, 9, 27, 12, tzinfo=UTC)
    snapshot = _empty_snapshot(start, end)
    engine = type("Engine", (), {"dispose": lambda self: None})()
    observed: dict[str, object] = {}

    class FakeSession:
        def __enter__(self) -> FakeSession:
            return self

        def __exit__(self, *_args: object) -> None:
            return None

        def scalar(self, _statement: object) -> object:
            return owner_id

    class FakeObservationService:
        def __init__(self, _session: FakeSession) -> None:
            pass

        def reliability_snapshot(self, **kwargs: object) -> JobReliabilitySnapshot:
            observed.update(kwargs)
            return snapshot

    monkeypatch.setattr(job_commands, "get_settings", lambda: object())
    monkeypatch.setattr(job_commands, "create_db_engine", lambda _settings: engine)
    monkeypatch.setattr(
        job_commands, "create_session_factory", lambda _engine: lambda: FakeSession()
    )
    monkeypatch.setattr(job_commands, "JobObservationService", FakeObservationService)

    result = CliRunner().invoke(
        app,
        [
            "jobs",
            "reliability-snapshot",
            "--window-start",
            start.isoformat(),
            "--window-end",
            end.isoformat(),
        ],
    )

    assert result.exit_code == 0
    payload = json.loads(result.stdout)
    assert payload["owner_id"] == str(owner_id)
    assert payload["total_jobs"] == 0
    assert payload["on_time_rate"] is None
    assert observed == {
        "owner_id": owner_id,
        "window_start": start,
        "window_end": end,
    }


def test_job_reliability_cli_rejects_timestamps_without_timezone(monkeypatch) -> None:
    engine_opened = False

    def create_engine(_settings: object) -> object:
        nonlocal engine_opened
        engine_opened = True
        return object()

    monkeypatch.setattr(job_commands, "get_settings", lambda: object())
    monkeypatch.setattr(job_commands, "create_db_engine", create_engine)

    result = CliRunner().invoke(
        app,
        [
            "jobs",
            "reliability-snapshot",
            "--window-start",
            "2026-09-24T12:00:00",
            "--window-end",
            "2026-09-27T12:00:00Z",
        ],
    )

    assert result.exit_code == 2
    assert engine_opened is False
