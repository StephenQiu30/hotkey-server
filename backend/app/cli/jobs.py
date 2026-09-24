from __future__ import annotations

import json
from datetime import UTC, datetime
from typing import Annotated

import typer
from sqlalchemy import select

from core.config import get_settings
from db.session import create_db_engine, create_session_factory
from identity.models import IdentityUser
from jobs.services import JobObservationService

jobs_app = typer.Typer(no_args_is_help=True)


def _parse_utc_datetime(value: str, *, option: str) -> datetime:
    try:
        parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as error:
        raise typer.BadParameter(
            "expected an ISO 8601 timestamp with a timezone", param_hint=option
        ) from error
    if parsed.utcoffset() is None:
        raise typer.BadParameter("timestamp must include a timezone", param_hint=option)
    return parsed.astimezone(UTC)


@jobs_app.command("reliability-snapshot")
def reliability_snapshot(
    window_start: Annotated[
        str,
        typer.Option(help="ISO 8601 observation-window start, including timezone."),
    ],
    window_end: Annotated[
        str,
        typer.Option(help="ISO 8601 observation-window end, including timezone."),
    ],
) -> None:
    """Print a read-only, reproducible webpage.collect reliability snapshot as JSON."""
    start = _parse_utc_datetime(window_start, option="--window-start")
    end = _parse_utc_datetime(window_end, option="--window-end")
    if end <= start:
        raise typer.BadParameter("window end must follow window start", param_hint="--window-end")

    engine = create_db_engine(get_settings())
    try:
        sessions = create_session_factory(engine)
        with sessions() as session:
            owner_id = session.scalar(select(IdentityUser.id))
            if owner_id is None:
                typer.echo("Reliability snapshot failed: identity_uninitialized", err=True)
                raise typer.Exit(code=1)
            snapshot = JobObservationService(session).reliability_snapshot(
                owner_id=owner_id,
                window_start=start,
                window_end=end,
            )
    finally:
        engine.dispose()

    payload = snapshot.model_dump(mode="json")
    payload["owner_id"] = str(owner_id)
    payload["on_time_rate"] = snapshot.on_time_rate
    typer.echo(json.dumps(payload, allow_nan=False, separators=(",", ":"), sort_keys=True))
