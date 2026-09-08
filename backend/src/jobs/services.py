from uuid import UUID

from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from audit.services import audit
from core.errors import AppError
from jobs.execution import cancel, enqueue
from jobs.models import Job
from jobs.schemas import JobPage, JobView


class JobService:
    def __init__(self, factory: sessionmaker[Session]):
        self.factory = factory

    def create_job(self, key: str) -> JobView:
        with self.factory.begin() as session:
            job = enqueue(session, key)
            audit(session, "diagnostic_requested", str(job.id))
            return JobView.model_validate(job)

    def jobs(self, limit: int, cursor: UUID | None) -> JobPage:
        with self.factory() as session:
            query = select(Job).order_by(Job.id)
            if cursor:
                query = query.where(Job.id > cursor)
            rows = list(session.scalars(query.limit(limit + 1)))
            return JobPage(
                items=[JobView.model_validate(v) for v in rows[:limit]],
                next_cursor=rows[limit - 1].id if len(rows) > limit else None,
            )

    def job(self, identity: UUID) -> JobView:
        with self.factory() as session:
            job = session.get(Job, identity)
            if not job:
                raise AppError("job_not_found", 404)
            return JobView.model_validate(job)

    def cancel_job(self, identity: UUID) -> JobView:
        self.job(identity)
        cancel(self.factory, identity)
        return self.job(identity)
