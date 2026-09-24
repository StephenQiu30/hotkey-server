from __future__ import annotations

import multiprocessing
import time
from collections.abc import Callable
from dataclasses import dataclass
from enum import StrEnum
from multiprocessing.connection import Connection
from multiprocessing.process import BaseProcess
from threading import Event


class JobProcessOutcome(StrEnum):
    COMPLETED = "completed"
    CANCELLED = "cancelled"
    TIMED_OUT = "timed_out"


@dataclass(frozen=True, slots=True)
class IsolatedProcessResult:
    outcome: JobProcessOutcome
    value: object | None = None


@dataclass(frozen=True, slots=True)
class _ChildMessage:
    kind: str
    value: object | None = None
    exception_type: str | None = None


class JobProcessCrashedError(RuntimeError):
    def __init__(self, exception_type: str) -> None:
        self.exception_type = exception_type
        super().__init__(f"job child exited without a typed result ({exception_type})")


class JobProcessShutdownError(RuntimeError):
    """The Worker is stopping before the current Kafka message can be acknowledged."""


class JobProcessTerminationError(RuntimeError):
    """The child process remained alive after terminate and kill escalation."""


class JobProcessSupervisor:
    def __init__(
        self,
        *,
        startup_timeout_seconds: float,
        execution_timeout_seconds: float,
        terminate_grace_seconds: float,
        poll_interval_seconds: float = 0.5,
    ) -> None:
        if (
            min(
                startup_timeout_seconds,
                execution_timeout_seconds,
                terminate_grace_seconds,
                poll_interval_seconds,
            )
            <= 0
        ):
            raise ValueError("process supervision timeouts must be positive")
        self._startup_timeout_seconds = startup_timeout_seconds
        self._execution_timeout_seconds = execution_timeout_seconds
        self._terminate_grace_seconds = terminate_grace_seconds
        self._poll_interval_seconds = poll_interval_seconds

    def run(
        self,
        target: Callable[..., object],
        args: tuple[object, ...],
        *,
        cancellation_requested: Callable[[], bool],
        stopping: Event,
    ) -> IsolatedProcessResult:
        context = multiprocessing.get_context("spawn")
        receiver, sender = context.Pipe(duplex=False)
        process = context.Process(
            target=_run_child,
            args=(sender, target, args),
        )
        process.daemon = True
        started_at = time.monotonic()
        startup_deadline = started_at + self._startup_timeout_seconds
        execution_deadline = startup_deadline + self._execution_timeout_seconds
        ready = False

        try:
            process.start()
            sender.close()
            while True:
                if stopping.is_set():
                    self._stop_process(process)
                    raise JobProcessShutdownError

                if cancellation_requested():
                    self._stop_process(process)
                    return IsolatedProcessResult(outcome=JobProcessOutcome.CANCELLED)

                if receiver.poll(0):
                    try:
                        message = receiver.recv()
                    except EOFError as error:
                        raise JobProcessCrashedError("ChildExitedWithoutResult") from error
                    if not isinstance(message, _ChildMessage):
                        self._stop_process(process)
                        raise JobProcessCrashedError("InvalidChildMessage")
                    if message.kind == "ready":
                        if time.monotonic() >= startup_deadline:
                            self._stop_process(process)
                            return IsolatedProcessResult(outcome=JobProcessOutcome.TIMED_OUT)
                        ready = True
                    elif message.kind == "result":
                        if time.monotonic() >= execution_deadline:
                            self._stop_process(process)
                            return IsolatedProcessResult(outcome=JobProcessOutcome.TIMED_OUT)
                        self._stop_process(process)
                        return IsolatedProcessResult(
                            outcome=JobProcessOutcome.COMPLETED,
                            value=message.value,
                        )
                    elif message.kind == "crashed":
                        exception_type = message.exception_type or "UnknownChildError"
                        self._stop_process(process)
                        raise JobProcessCrashedError(exception_type)
                    else:
                        self._stop_process(process)
                        raise JobProcessCrashedError("InvalidChildMessage")

                if process.exitcode is not None:
                    raise JobProcessCrashedError("ChildExitedWithoutResult")

                now = time.monotonic()
                deadline = execution_deadline if ready else startup_deadline
                if now >= deadline:
                    self._stop_process(process)
                    return IsolatedProcessResult(outcome=JobProcessOutcome.TIMED_OUT)
                receiver.poll(min(self._poll_interval_seconds, deadline - now))
        except BaseException:
            if process.pid is not None and process.is_alive():
                self._stop_process(process)
            raise
        finally:
            receiver.close()
            sender.close()
            if process.pid is not None and not process.is_alive():
                process.join()
                process.close()

    def _stop_process(self, process: BaseProcess) -> None:
        if process.pid is None:
            return
        if process.is_alive():
            process.terminate()
            process.join(self._terminate_grace_seconds)
        if process.is_alive():
            process.kill()
            process.join(self._terminate_grace_seconds)
        if process.is_alive():
            raise JobProcessTerminationError("job child did not stop after kill escalation")
        process.join()


def _run_child(
    sender: Connection,
    target: Callable[..., object],
    args: tuple[object, ...],
) -> None:
    try:
        sender.send(_ChildMessage(kind="ready"))
        try:
            value = target(*args)
        except BaseException as error:
            sender.send(_ChildMessage(kind="crashed", exception_type=type(error).__name__))
        else:
            sender.send(_ChildMessage(kind="result", value=value))
    except (BrokenPipeError, EOFError, OSError):
        return
    finally:
        sender.close()
