from datetime import timedelta
from uuid import uuid4

import pytest
from sqlalchemy import func, select

from core.clock import utcnow
from jobs.contracts import Dispatch
from jobs.execution import cancel, claim, complete, enqueue, reconcile
from jobs.models import Job, JobResult, Outbox

pytestmark = pytest.mark.integration


def test_transaction_rolls_back_job_and_outbox(database):
    with pytest.raises(RuntimeError), database.begin() as session:
        enqueue(session, "rollback")
        raise RuntimeError("interrupt before commit")
    with database() as session:
        assert session.scalar(select(func.count()).select_from(Job)) == 0
        assert session.scalar(select(func.count()).select_from(Outbox)) == 0


def test_idempotent_enqueue_and_duplicate_delivery(database):
    with database.begin() as session:
        job = enqueue(session, "same")
        job_id = job.id
    with database.begin() as session:
        assert enqueue(session, "same").id == job_id
    dispatch = Dispatch(job_id=job_id, epoch=1)
    lease = claim(database, dispatch)
    assert lease is not None
    assert claim(database, dispatch) is None
    assert complete(database, lease)
    assert not complete(database, lease)
    assert claim(database, dispatch) is None
    with database() as session:
        assert session.scalar(select(func.count()).select_from(JobResult)) == 1


def test_collection_job_kind_uses_same_ledger_without_diagnostic_result(database):
    with database.begin() as session:
        job = enqueue(session, "collection:one", kind="collect_page")
        job_id = job.id
    lease = claim(database, Dispatch(job_id=job_id, epoch=1))
    assert lease is not None and lease.kind == "collect_page"
    assert complete(database, lease)
    with database() as session:
        assert session.get(Job, job_id).status == "succeeded"
        assert session.get(JobResult, job_id) is None


def test_expired_lease_fences_old_result_and_caps_attempts(database):
    with database.begin() as session:
        job_id = enqueue(session, "recovery").id
    lease = claim(database, Dispatch(job_id=job_id, epoch=1))
    assert lease is not None
    with database.begin() as session:
        job = session.get(Job, job_id)
        job.lease_until = utcnow() - timedelta(seconds=1)
    assert not complete(database, lease)
    assert reconcile(database) == 1
    assert claim(database, Dispatch(job_id=job_id, epoch=1)) is None
    with database.begin() as session:
        job = session.get(Job, job_id)
        job.available_at = utcnow() - timedelta(seconds=1)
    replacement = claim(database, Dispatch(job_id=job_id, epoch=2))
    assert replacement is not None
    assert not complete(database, lease)
    assert complete(database, replacement)


def test_cancelled_job_cannot_commit(database):
    with database.begin() as session:
        job_id = enqueue(session, "cancel").id
    lease = claim(database, Dispatch(job_id=job_id, epoch=1))
    assert lease is not None
    assert cancel(database, job_id)
    assert not complete(database, lease)
    assert reconcile(database) == 0


def test_sent_without_consumer_is_republished(database):
    with database.begin() as session:
        job = enqueue(session, "lost-message")
        job.available_at = utcnow() - timedelta(minutes=5)
        outbox = session.scalar(select(Outbox).where(Outbox.job_id == job.id))
        outbox.sent_at = utcnow() - timedelta(minutes=5)
    assert reconcile(database) == 1
    with database() as session:
        assert session.scalar(select(func.count()).select_from(Outbox)) == 2


def test_max_attempts_stops_poison_job(database):
    with database.begin() as session:
        job = enqueue(session, "poison")
        job.attempts = job.max_attempts
        job_id = job.id
    assert claim(database, Dispatch(job_id=job_id, epoch=1)) is None
    with database() as session:
        assert session.get(Job, job_id).status == "failed"


def test_missing_job_is_ignored(database):
    assert claim(database, Dispatch(job_id=uuid4(), epoch=1)) is None


def test_concurrent_enqueues_have_one_intent(database):
    from concurrent.futures import ThreadPoolExecutor

    def insert_one(_):
        with database.begin() as session:
            return enqueue(session, "concurrent").id

    with ThreadPoolExecutor(max_workers=4) as pool:
        ids = list(pool.map(insert_one, range(8)))
    assert len(set(ids)) == 1
    with database() as session:
        assert session.scalar(select(func.count()).select_from(Outbox)) == 1


def test_migration_matches_models(database):
    from alembic.autogenerate import compare_metadata
    from alembic.migration import MigrationContext

    from db.metadata import Base

    with database.kw["bind"].connect() as connection:
        assert compare_metadata(MigrationContext.configure(connection), Base.metadata) == []
