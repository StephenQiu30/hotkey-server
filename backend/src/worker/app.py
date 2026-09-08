from celery.signals import worker_process_init, worker_process_shutdown

from core.config import Settings
from db.session import Database
from jobs.contracts import Dispatch
from jobs.execution import claim, complete
from worker.messaging import TASK_NAME, celery_app

settings = Settings()
app = celery_app(settings)
_database: Database | None = None


def initialize(**kwargs: object) -> None:
    global _database
    _database = Database(settings)


def shutdown(**kwargs: object) -> None:
    global _database
    if _database is not None:
        _database.close()
        _database = None


def verify_pipeline(payload: dict[str, object]) -> None:
    message = Dispatch.model_validate(payload)
    if _database is None:
        raise RuntimeError("worker_process_not_initialized; use prefork pool")
    lease = claim(_database.sessions, message, settings.lease_seconds)
    if lease is not None:
        complete(_database.sessions, lease)


worker_process_init.connect(initialize, weak=False)
worker_process_shutdown.connect(shutdown, weak=False)
app.task(name=TASK_NAME)(verify_pipeline)
