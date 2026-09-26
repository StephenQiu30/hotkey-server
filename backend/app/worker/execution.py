from __future__ import annotations

import ast
import multiprocessing
import re
import time
from collections.abc import Callable
from dataclasses import dataclass
from enum import StrEnum
from json import JSONDecodeError
from multiprocessing.connection import Connection
from multiprocessing.process import BaseProcess
from pathlib import Path
from threading import Event
from xml.etree.ElementTree import ParseError

import httpx
from pydantic import ValidationError


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
    error_code: str | None = None
    safe_summary: str | None = None
    error_message: str | None = None


class JobProcessCrashedError(RuntimeError):
    def __init__(self, exception_type: str) -> None:
        self.exception_type = exception_type
        super().__init__(f"job child exited without a typed result ({exception_type})")


class JobProcessChildError(RuntimeError):
    """An exception reported by a live child with only a vetted literal message."""

    def __init__(
        self,
        *,
        exception_type: str,
        error_code: str,
        safe_summary: str,
        error_message: str | None = None,
    ) -> None:
        self.exception_type = exception_type
        self.error_code = error_code
        self.safe_summary = safe_summary
        self.error_message = error_message
        super().__init__(f"{exception_type}: {safe_summary}")


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
        execution_timeout_seconds: float | None = None,
    ) -> IsolatedProcessResult:
        execution_timeout = (
            self._execution_timeout_seconds
            if execution_timeout_seconds is None
            else execution_timeout_seconds
        )
        if execution_timeout <= 0:
            raise ValueError("process execution timeout must be positive")
        context = multiprocessing.get_context("spawn")
        receiver, sender = context.Pipe(duplex=False)
        process = context.Process(
            target=_run_child,
            args=(sender, target, args),
        )
        process.daemon = True
        started_at = time.monotonic()
        startup_deadline = started_at + self._startup_timeout_seconds
        execution_deadline = startup_deadline + execution_timeout
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
                    elif message.kind == "failed":
                        self._stop_process(process)
                        if (
                            message.exception_type is None
                            or message.error_code
                            not in {
                                "invalid_response",
                                "parse_error",
                                "network_error",
                                "job_unhandled_exception",
                            }
                            or message.safe_summary is None
                            or (
                                message.error_message is not None
                                and (
                                    not isinstance(message.error_message, str)
                                    or len(message.error_message) > 200
                                )
                            )
                        ):
                            raise JobProcessCrashedError("InvalidChildMessage")
                        raise JobProcessChildError(
                            exception_type=message.exception_type,
                            error_code=message.error_code,
                            safe_summary=message.safe_summary,
                            error_message=message.error_message,
                        )
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
        except Exception as error:
            error_code, safe_summary = _classify_child_exception(error)
            exception_type = type(error).__name__
            if re.fullmatch(r"[A-Za-z][A-Za-z0-9_]{0,127}", exception_type) is None:
                exception_type = "UnknownChildError"
            sender.send(
                _ChildMessage(
                    kind="failed",
                    exception_type=exception_type,
                    error_code=error_code,
                    safe_summary=safe_summary,
                    error_message=_owned_literal_error_message(error),
                )
            )
        else:
            sender.send(_ChildMessage(kind="result", value=value))
    except (BrokenPipeError, EOFError, OSError):
        return
    finally:
        sender.close()


def _classify_child_exception(error: Exception) -> tuple[str, str]:
    if isinstance(error, (JSONDecodeError, UnicodeError, ParseError, SyntaxError)):
        return "parse_error", "子进程解析数据失败"
    if isinstance(error, (ValidationError, ValueError)):
        return "invalid_response", "子进程数据校验失败"
    if isinstance(error, (httpx.TransportError, TimeoutError, ConnectionError)):
        return "network_error", "子进程访问外部服务失败"
    return "job_unhandled_exception", "子进程执行失败"


def _owned_literal_error_message(error: Exception) -> str | None:
    """Expose only a literal raised by our application, never third-party or input text."""
    if type(error) not in (ValueError, TypeError):
        return None
    trace = error.__traceback__
    if trace is None:
        return None
    while trace.tb_next is not None:
        trace = trace.tb_next
    path = Path(trace.tb_frame.f_code.co_filename).resolve()
    if not path.is_relative_to(Path(__file__).resolve().parents[1]) or path.suffix != ".py":
        return None
    try:
        source = ast.parse(path.read_text(encoding="utf-8"))
    except (OSError, SyntaxError, UnicodeError):
        return None
    for node in ast.walk(source):
        if not isinstance(node, ast.Raise) or not (
            node.lineno <= trace.tb_lineno <= (node.end_lineno or node.lineno)
        ):
            continue
        call = node.exc
        if (
            isinstance(call, ast.Call)
            and isinstance(call.func, ast.Name)
            and call.func.id == type(error).__name__
            and len(call.args) == 1
            and not call.keywords
            and isinstance(call.args[0], ast.Constant)
            and isinstance(call.args[0].value, str)
            and str(error) == call.args[0].value
        ):
            return call.args[0].value[:200]
    return None
