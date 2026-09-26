from __future__ import annotations

import os
import signal
import time
from collections.abc import Callable
from contextlib import contextmanager
from datetime import UTC, datetime, timedelta
from json import JSONDecodeError
from pathlib import Path
from threading import Event
from types import SimpleNamespace
from typing import Any
from uuid import uuid4

import pytest
from pydantic import BaseModel, Field

from content.discovery import _topic_id_from_configuration_ref
from jobs.execution import ExecutionLease
from jobs.schemas import JobFailureCategory
from worker import app as worker_app
from worker.execution import (
    JobProcessChildError,
    JobProcessCrashedError,
    JobProcessOutcome,
    JobProcessShutdownError,
    JobProcessSupervisor,
)
from worker.messaging import MessageDeferredError


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


class _VersionInput(BaseModel):
    component_version: str = Field(pattern=r"^[a-z]+$")


def _raise_validation_error() -> None:
    _VersionInput(component_version="secret/invalid")


def _raise_parse_error() -> None:
    raise JSONDecodeError("secret-token", "private payload", 0)


def _raise_value_error() -> None:
    raise ValueError("secret invalid value")


def _raise_owned_value_error() -> None:
    _topic_id_from_configuration_ref("invalid")


def _raise_network_error() -> None:
    raise ConnectionError("https://example.invalid/?token=private")


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


def test_run_can_override_the_default_execution_deadline(tmp_path: Path) -> None:
    started = tmp_path / "started"
    finished = tmp_path / "finished"

    supervisor = JobProcessSupervisor(
        startup_timeout_seconds=2.0,
        execution_timeout_seconds=10.0,
        terminate_grace_seconds=0.05,
        poll_interval_seconds=0.01,
    )
    result = supervisor.run(
        _wait_and_mark,
        (str(started), str(finished), 3.0),
        cancellation_requested=lambda: False,
        stopping=Event(),
        execution_timeout_seconds=0.1,
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


@pytest.mark.parametrize(
    ("target", "error_code", "exception_type"),
    [
        (_raise_validation_error, "invalid_response", "ValidationError"),
        (_raise_value_error, "invalid_response", "ValueError"),
        (_raise_owned_value_error, "invalid_response", "ValueError"),
        (_raise_parse_error, "parse_error", "JSONDecodeError"),
        (_raise_network_error, "network_error", "ConnectionError"),
        (_raise_with_private_message, "job_unhandled_exception", "RuntimeError"),
    ],
)
def test_child_exception_returns_safe_typed_failure_before_deadline(
    target: Callable[[], object],
    error_code: str,
    exception_type: str,
) -> None:
    started_at = time.monotonic()
    with pytest.raises(JobProcessChildError) as captured:
        _supervisor(execution_timeout_seconds=10).run(
            target,
            (),
            cancellation_requested=lambda: False,
            stopping=Event(),
        )

    assert time.monotonic() - started_at < 3
    assert captured.value.error_code == error_code
    assert captured.value.exception_type == exception_type
    assert "private" not in captured.value.safe_summary
    assert "private" not in str(captured.value)
    assert "token" not in str(captured.value)
    assert captured.value.error_message == (
        "keyword search requires a topic configuration"
        if target is _raise_owned_value_error
        else None
    )


def test_process_exit_without_result_remains_a_crash() -> None:
    with pytest.raises(JobProcessCrashedError):
        _supervisor().run(
            _exit_without_result,
            (),
            cancellation_requested=lambda: False,
            stopping=Event(),
        )


def test_unreported_process_crash_still_defers_to_lease_expiration(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    retry_at = datetime.now(UTC) + timedelta(seconds=60)
    monkeypatch.setattr(worker_app, "_lease_retry_time", lambda *_args, **_kwargs: retry_at)

    with pytest.raises(MessageDeferredError) as captured:
        worker_app._defer_after_child_exit(
            lambda: None,
            lease=ExecutionLease(
                job_id=uuid4(),
                worker_id="worker-test",
                epoch=1,
                expires_at=retry_at,
                checkpoint_sequence=0,
                checkpoint={},
            ),
            lease_seconds=60,
            clock=None,
            cause=JobProcessCrashedError("ChildExitedWithoutResult"),
            message=SimpleNamespace(kind="keyword.search", source_key=None),
        )

    assert captured.value.retry_at == retry_at


@pytest.mark.parametrize(
    ("error_code", "category", "automatic_retry"),
    [
        ("invalid_response", JobFailureCategory.INVALID_RESPONSE, False),
        ("parse_error", JobFailureCategory.PARSE_ERROR, False),
        ("network_error", JobFailureCategory.TRANSIENT, True),
        ("job_unhandled_exception", JobFailureCategory.CONFIGURATION_UNAVAILABLE, False),
    ],
)
def test_child_exception_failure_policy(
    error_code: str, category: JobFailureCategory, automatic_retry: bool
) -> None:
    now = datetime.now(UTC)
    failure = worker_app._child_exception_failure(
        JobProcessChildError(
            exception_type="ValidationError",
            error_code=error_code,
            safe_summary="安全摘要",
        ),
        occurred_at=now,
    )

    assert failure.error_code == error_code
    assert failure.category is category
    assert (failure.retry_at is not None) is automatic_retry
    assert (failure.max_attempts is not None) is automatic_retry


@pytest.mark.parametrize(
    ("error_message", "exception_type"),
    [(None, "ValidationError"), ("固定消息" * 80, "ValueError"), ("外部消息", "ValidationError")],
)
def test_reported_child_exception_finalizes_without_deferred_replay(
    monkeypatch: pytest.MonkeyPatch,
    error_message: str | None,
    exception_type: str,
) -> None:
    now = datetime.now(UTC)
    lease = ExecutionLease(
        job_id=uuid4(),
        worker_id="worker-test",
        epoch=1,
        expires_at=now + timedelta(seconds=60),
        checkpoint_sequence=0,
        checkpoint={},
    )
    body = SimpleNamespace(
        operation_id=uuid4(),
        job_id=lease.job_id,
        kind="keyword.search",
        configuration_version=1,
        source_key="bilibili",
        source_capability=None,
        message_id=uuid4(),
    )
    reference = object()
    finalized: list[Any] = []
    logged: list[dict[str, str]] = []
    deadlines: list[tuple[str, str | None]] = []

    class FakeExecutionService:
        def __init__(self, *_args: object, **_kwargs: object) -> None:
            pass

        def is_processed(self, _message_id: object) -> bool:
            return False

        def acknowledge_cancelled(self, **_kwargs: object) -> bool:
            return False

        def acquire(self, **_kwargs: object) -> ExecutionLease:
            return lease

    class FakeSupervisor:
        def run(self, *_args: object, **_kwargs: object) -> None:
            raise JobProcessChildError(
                exception_type=exception_type,
                error_code="invalid_response",
                safe_summary="子进程数据校验失败",
                error_message=error_message,
            )

    class FakeLogger:
        def error(self, _event: str, **fields: str) -> None:
            logged.append(fields)

    @contextmanager
    def sessions() -> Any:
        yield object()

    monkeypatch.setattr(worker_app, "decode_job_message", lambda _message: (body, reference))
    monkeypatch.setattr(worker_app, "JobExecutionService", FakeExecutionService)
    monkeypatch.setattr(
        worker_app,
        "_finalize_supervised_result",
        lambda *_args, **kwargs: finalized.append(kwargs),
    )
    monkeypatch.setattr(worker_app.structlog, "get_logger", lambda _name: FakeLogger())

    handler = worker_app.create_job_message_handler(
        sessions,
        {"keyword.search": lambda _context: None},
        worker_id="worker-test",
        lease_seconds=60,
        clock=lambda: now,
        supervisor=FakeSupervisor(),
        stopping=Event(),
        job_execution_timeout_seconds=lambda kind, source_key: (
            deadlines.append((kind, source_key)) or 240
        ),
    )
    handler(object())

    assert len(finalized) == 1
    failure = finalized[0]["report"].failure.to_failure()
    assert failure.error_code == "invalid_response"
    assert failure.category is JobFailureCategory.INVALID_RESPONSE
    assert failure.retry_at is None
    assert deadlines == [("keyword.search", "bilibili")]
    expected_log = {
        "error_code": "invalid_response",
        "exception_type": exception_type,
        "failure_category": "invalid_response",
    }
    if error_message is not None and exception_type in {"ValueError", "TypeError"}:
        expected_log["error_message"] = error_message[:200]
    assert logged == [expected_log]


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
