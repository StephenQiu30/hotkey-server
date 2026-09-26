from __future__ import annotations

from datetime import UTC, date, datetime
from types import SimpleNamespace
from unittest.mock import MagicMock
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient
from sqlalchemy.dialects import postgresql

from api.dependencies import get_report_service, require_identity_session
from core.config import Settings
from core.errors import ApplicationError
from main import create_app
from reports.schemas import ReportKind
from reports.services import ReportService


def _session() -> MagicMock:
    return MagicMock()


def test_report_list_scopes_latest_final_versions_and_date_window() -> None:
    session = _session()
    session.scalars.return_value.all.return_value = []
    owner_id = uuid4()
    topic_id = uuid4()

    items, cursor = ReportService(session).list_reports(
        owner_id=owner_id,
        topic_id=topic_id,
        date_from=date(2026, 9, 24),
        date_to=date(2026, 9, 25),
        kind=ReportKind.DAILY,
        cursor=None,
        limit=20,
    )

    assert items == []
    assert cursor is None
    statement = session.scalars.call_args.args[0]
    compiled = statement.compile(dialect=postgresql.dialect())
    sql = str(compiled)
    assert "reports.owner_id" in sql
    assert "reports.status" in sql
    assert "reports.version = (SELECT" in sql
    assert "ORDER BY reports.window_start DESC, reports.id DESC" in sql
    assert owner_id in compiled.params.values()
    assert topic_id in compiled.params.values()
    assert datetime(2026, 9, 23, 16, tzinfo=UTC) in compiled.params.values()
    assert datetime(2026, 9, 25, 16, tzinfo=UTC) in compiled.params.values()


def test_report_detail_missing_or_foreign_owner_is_not_found() -> None:
    session = _session()
    session.scalars.return_value.first.return_value = None
    owner_id = uuid4()
    report_id = uuid4()

    with pytest.raises(ApplicationError, match="resource_not_found"):
        ReportService(session).get_report(owner_id=owner_id, report_id=report_id)

    statement = session.scalars.call_args.args[0]
    compiled = statement.compile(dialect=postgresql.dialect())
    assert owner_id in compiled.params.values()
    assert report_id in compiled.params.values()
    assert "reports.status" in str(compiled)


def test_report_routes_apply_session_identity_and_public_error_contract() -> None:
    owner_id = uuid4()
    app = create_app(
        Settings(
            environment="test",
            log_level="WARNING",
            database_url="postgresql+psycopg://unused:unused@127.0.0.1/unopened",
        )
    )
    service = SimpleNamespace(
        list_reports=lambda **kwargs: ([], None),
        get_report=lambda **kwargs: (_ for _ in ()).throw(ApplicationError("resource_not_found")),
    )
    app.dependency_overrides[require_identity_session] = lambda: SimpleNamespace(
        view=SimpleNamespace(user=SimpleNamespace(id=owner_id))
    )
    app.dependency_overrides[get_report_service] = lambda: service
    client = TestClient(app)

    listed = client.get("/api/v1/reports")
    assert listed.status_code == 200
    assert listed.json() == {"items": [], "next_cursor": None}
    assert listed.headers["cache-control"] == "no-store"

    hidden = client.get(f"/api/v1/reports/{uuid4()}")
    assert hidden.status_code == 404
    assert hidden.json()["code"] == "resource_not_found"

    invalid = client.get("/api/v1/reports?date_from=2026-09-25&date_to=2026-09-24")
    assert invalid.status_code == 422
    assert invalid.json()["code"] == "validation_error"


def test_report_cursor_cannot_cross_owner_or_filter_boundary() -> None:
    session = _session()
    session.scalars.return_value.first.return_value = None
    with pytest.raises(ApplicationError, match="resource_not_found"):
        ReportService(session).list_reports(
            owner_id=uuid4(),
            topic_id=uuid4(),
            date_from=None,
            date_to=None,
            kind=ReportKind.DAILY,
            cursor=uuid4(),
            limit=20,
        )
