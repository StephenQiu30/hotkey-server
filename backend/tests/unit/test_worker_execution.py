from __future__ import annotations

import os
import signal
import time
from collections.abc import Callable
from pathlib import Path
from threading import Event

import pytest

from worker.execution import (
    JobProcessCrashedError,
    JobProcessOutcome,
    JobProcessShutdownError,
    JobProcessSupervisor,
)


def _return_value(value: str) -> str:
    return value


def _wait_and_mark(started_path: str, finished_path: str, seconds: float) -> str:
    Path(started_path).touch()
    time.sleep(seconds)
    Path(finished_path).touch()
    return "finished"


def _ignore_terminate_and_wait(started_path: str) -> None:
    signal.signal(signal.SIGTERM, signal.SIG_IGN)
    Path(started_path).touch()
    while True:
        time.sleep(0.02)


def _raise_with_private_message() -> None:
    raise RuntimeError("https://example.invalid/?token=private")


def _exit_without_result() -> None:
    os._exit(19)


def _supervisor(*, execution_timeout_seconds: float = 2.0) -> JobProcessSupervisor:
    return JobProcessSupervisor(
        startup_timeout_seconds=2.0,
        execution_timeout_seconds=execution_timeout_seconds,
        terminate_grace_seconds=0.05,
        poll_interval_seconds=0.01,
    )


def test_spawned_job_returns_typed_process_result() -> None:
    result = _supervisor().run(
        _return_value,
        ("finished",),
        cancellation_requested=lambda: False,
        stopping=Event(),
    )

    assert result.outcome is JobProcessOutcome.COMPLETED
    assert result.value == "finished"


def test_hard_deadline_terminates_handler_that_never_returns(tmp_path: Path) -> None:
    started = tmp_path / "started"
    finished = tmp_path / "finished"

    result = _supervisor(execution_timeout_seconds=0.1).run(
        _wait_and_mark,
        (str(started), str(finished), 10.0),
        cancellation_requested=lambda: False,
        stopping=Event(),
    )

    assert result.outcome is JobProcessOutcome.TIMED_OUT
    assert started.exists()
    assert not finished.exists()


def test_cancellation_kills_child_which_ignores_terminate(tmp_path: Path) -> None:
    started = tmp_path / "started"

    result = _supervisor().run(
        _ignore_terminate_and_wait,
        (str(started),),
        cancellation_requested=lambda: started.exists(),
        stopping=Event(),
    )

    assert result.outcome is JobProcessOutcome.CANCELLED
    assert started.exists()


@pytest.mark.parametrize("target", [_raise_with_private_message, _exit_without_result])
def test_unclassified_child_exit_exposes_no_exception_message(
    target: Callable[[], object],
) -> None:
    with pytest.raises(JobProcessCrashedError) as captured:
        _supervisor().run(
            target,
            (),
            cancellation_requested=lambda: False,
            stopping=Event(),
        )

    assert "private" not in str(captured.value)
    assert "token" not in str(captured.value)


def test_worker_stop_reaps_child_without_returning_a_committable_result(
    tmp_path: Path,
) -> None:
    started = tmp_path / "started"
    stopping = Event()

    def stopping_requested() -> bool:
        if started.exists():
            stopping.set()
        return False

    with pytest.raises(JobProcessShutdownError):
        _supervisor().run(
            _ignore_terminate_and_wait,
            (str(started),),
            cancellation_requested=stopping_requested,
            stopping=stopping,
        )

    assert started.exists()
