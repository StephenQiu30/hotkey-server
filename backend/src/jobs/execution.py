from datetime import datetime, timedelta
from uuid import UUID, uuid4

from sqlalchemy import and_, or_, select
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.orm import Session, sessionmaker

from core.clock import utcnow
from jobs.contracts import Dispatch, Lease
from jobs.models import Attempt, Job, JobResult, Outbox


def enqueue(session: Session, key: str) -> Job:
    if not key.strip() or len(key) > 128:
        raise ValueError("idempotency key must contain 1-128 characters")
    now = utcnow()
    job_id = session.scalar(
        insert(Job)
        .values(
            id=uuid4(),
            key=key,
            available_at=now,
            deadline=now + timedelta(hours=1),
        )
        .on_conflict_do_nothing(index_elements=[Job.key])
        .returning(Job.id)
    )
    if job_id is not None:
        session.add(Outbox(id=uuid4(), job_id=job_id, epoch=1, due_at=now))
    job = session.scalar(select(Job).where(Job.key == key))
    assert job is not None
    session.flush()
    return job


def claim(factory: sessionmaker[Session], message: Dispatch, seconds: int = 30) -> Lease | None:
    with factory.begin() as session:
        job = session.scalar(select(Job).where(Job.id == message.job_id).with_for_update())
        now = utcnow()
        if (
            job is None
            or job.epoch != message.epoch
            or job.status != "queued"
            or job.available_at > now
        ):
            return None
        if job.attempts >= job.max_attempts or job.deadline <= now:
            job.status = "failed"
            return None
        job.status = "running"
        job.attempts += 1
        job.fencing_token += 1
        job.lease_until = min(now + timedelta(seconds=seconds), job.deadline)
        session.add(
            Attempt(id=uuid4(), job_id=job.id, fencing_token=job.fencing_token, started_at=now)
        )
        return Lease(job.id, job.epoch, job.fencing_token)


def finish_attempt(session: Session, job: Job, outcome: str, now: datetime) -> None:
    attempt = session.scalar(
        select(Attempt).where(Attempt.job_id == job.id, Attempt.fencing_token == job.fencing_token)
    )
    if attempt is not None:
        attempt.ended_at = now
        attempt.outcome = outcome


def complete(factory: sessionmaker[Session], lease: Lease) -> bool:
    with factory.begin() as session:
        job = session.scalar(select(Job).where(Job.id == lease.job_id).with_for_update())
        now = utcnow()
        if (
            job is None
            or job.status != "running"
            or job.epoch != lease.epoch
            or job.fencing_token != lease.fencing_token
            or job.lease_until is None
            or job.lease_until <= now
            or job.deadline <= now
        ):
            return False
        # This diagnostic is an actual durable side effect, never labelled source collection.
        session.add(JobResult(job_id=job.id, completed_at=now, result="pipeline_verified"))
        job.status = "succeeded"
        job.completed_at = now
        job.lease_until = None
        finish_attempt(session, job, "succeeded", now)
        return True


def cancel(factory: sessionmaker[Session], job_id: UUID) -> bool:
    with factory.begin() as session:
        job = session.scalar(select(Job).where(Job.id == job_id).with_for_update())
        if job is None or job.status not in {"queued", "running"}:
            return False
        finish_attempt(session, job, "cancelled", utcnow())
        job.status = "cancelled"
        job.lease_until = None
        return True


def reconcile(factory: sessionmaker[Session], stale_seconds: int = 120) -> int:
    now = utcnow()
    changed = 0
    with factory.begin() as session:
        sent_stale = (
            select(Outbox.id)
            .where(
                Outbox.job_id == Job.id,
                Outbox.epoch == Job.epoch,
                Outbox.sent_at < now - timedelta(seconds=stale_seconds),
            )
            .exists()
        )
        jobs = session.scalars(
            select(Job)
            .where(
                or_(
                    and_(Job.status == "running", Job.lease_until <= now),
                    and_(Job.status == "queued", sent_stale),
                    and_(Job.status.in_(["queued", "running"]), Job.deadline <= now),
                )
            )
            .limit(100)
            .with_for_update(skip_locked=True)
        ).all()
        for job in jobs:
            finish_attempt(session, job, "lease_expired", now)
            job.lease_until = None
            changed += 1
            if job.attempts >= job.max_attempts or job.deadline <= now:
                job.status = "failed"
                continue
            job.status = "queued"
            job.epoch += 1
            job.available_at = now + timedelta(seconds=min(2**job.attempts, 30))
            session.add(Outbox(id=uuid4(), job_id=job.id, epoch=job.epoch, due_at=job.available_at))
    return changed
