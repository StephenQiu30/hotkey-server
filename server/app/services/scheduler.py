from __future__ import annotations

import asyncio
from contextlib import suppress
from datetime import datetime, timezone

from server.app.core.settings import settings
from server.app.db.session import SessionLocal
from server.app.services.check_runner import run_hotspot_check
from server.app.services.reports import (
    generate_and_send_report,
    previous_daily_period_start,
    previous_weekly_period_start,
)

_last_daily_report_date = None
_last_weekly_report_start = None


async def scheduler_loop() -> None:
    while True:
        await asyncio.sleep(max(settings.check_interval_minutes, 1) * 60)
        await asyncio.to_thread(_run_scheduled_check)


def start_scheduler() -> asyncio.Task | None:
    if not settings.scheduler_enabled:
        return None
    return asyncio.create_task(scheduler_loop())


async def stop_scheduler(task: asyncio.Task | None) -> None:
    if task is None:
        return
    task.cancel()
    with suppress(asyncio.CancelledError):
        await task


def _run_scheduled_check() -> None:
    with SessionLocal() as session:
        run_hotspot_check(session, trigger_type="scheduled")
        _maybe_run_daily_report(session)
        _maybe_run_weekly_report(session)


def _maybe_run_daily_report(session) -> None:
    global _last_daily_report_date
    if not settings.daily_report_enabled:
        return
    now = datetime.now(timezone.utc)
    if now.hour < settings.daily_report_hour:
        return
    report_date = previous_daily_period_start(now)
    if _last_daily_report_date == report_date:
        return
    generate_and_send_report(session, report_type="daily", period_start=report_date)
    session.commit()
    _last_daily_report_date = report_date


def _maybe_run_weekly_report(session) -> None:
    global _last_weekly_report_start
    if not settings.weekly_report_enabled:
        return
    now = datetime.now(timezone.utc)
    if now.isoweekday() != settings.weekly_report_weekday or now.hour < settings.weekly_report_hour:
        return
    report_start = previous_weekly_period_start(now)
    if _last_weekly_report_start == report_start:
        return
    generate_and_send_report(session, report_type="weekly", period_start=report_start)
    session.commit()
    _last_weekly_report_start = report_start
