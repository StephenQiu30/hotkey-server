import os

import pytest
from alembic import command
from sqlalchemy import create_engine, text
from sqlalchemy.engine import make_url
from sqlalchemy.orm import sessionmaker


@pytest.fixture
def database():
    url = os.getenv("HOTKEY_TEST_DATABASE_URL")
    if not url:
        pytest.skip("disposable PostgreSQL not configured")
    if make_url(url).database != "hotkey_test":
        pytest.fail("refusing to modify a database other than hotkey_test")
    from migrations.config import migration_config

    engine = create_engine(url)
    command.upgrade(migration_config(url), "head")
    with engine.begin() as conn:
        conn.execute(
            text(
                "TRUNCATE knowledge_chunks, knowledge_citations, knowledge_versions, "
                "knowledge_entries, "
                "analysis_labels, analysis_samples, analysis_runs, notifications, "
                "event_members, event_revisions, events, "
                "collection_checkpoints, "
                "collection_budget_usage, monitor_matches, "
                "content_observations, "
                "content_versions, contents, raw_pages, collection_runs, login_sessions, "
                "owners, monitor_versions, monitors, audit_events, job_results, outbox, "
                "job_attempts, jobs"
            )
        )
    yield sessionmaker(engine, expire_on_commit=False)
    engine.dispose()
