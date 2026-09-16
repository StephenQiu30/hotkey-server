from celery.signals import worker_process_init, worker_process_shutdown

from collection.execution import CollectionExecutor
from collection.services import CollectionService
from core.config import Settings
from db.session import Database
from evidence.adapters.minio import MinioEvidenceStore
from jobs.contracts import Dispatch
from jobs.execution import claim, complete
from sources.execution import PublicCollectionFetcher
from sources.services import SourceService
from worker.messaging import TASK_NAME, celery_app

settings = Settings()
app = celery_app(settings)
_database: Database | None = None
_collection_executor: CollectionExecutor | None = None


def initialize(**kwargs: object) -> None:
    global _collection_executor, _database
    _database = Database(settings)
    if settings.s3_configured:
        sources = SourceService.from_settings(settings)
        _collection_executor = CollectionExecutor(
            CollectionService(_database.sessions, sources, evidence_configured=True),
            sources,
            PublicCollectionFetcher(),
            MinioEvidenceStore.from_settings(settings),
        )


def shutdown(**kwargs: object) -> None:
    global _collection_executor, _database
    if _database is not None:
        _database.close()
        _database = None
    _collection_executor = None


def execute_job(payload: dict[str, object]) -> None:
    message = Dispatch.model_validate(payload)
    if _database is None:
        raise RuntimeError("worker_process_not_initialized; use prefork pool")
    lease = claim(_database.sessions, message, settings.lease_seconds)
    if lease is None:
        return
    if lease.kind == "collect_page":
        if _collection_executor is None:
            raise RuntimeError("evidence_store_not_configured")
        _collection_executor.execute(lease)
        return
    complete(_database.sessions, lease)


worker_process_init.connect(initialize, weak=False)
worker_process_shutdown.connect(shutdown, weak=False)
app.task(name=TASK_NAME)(execute_job)
