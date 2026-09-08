from typing import Any

from celery import Celery
from kombu import Exchange, Queue
from sqlalchemy import select
from sqlalchemy.orm import Session, sessionmaker

from core.clock import utcnow
from core.config import Settings
from jobs.contracts import Dispatch
from jobs.models import Job, Outbox

TASK_NAME = "hotkey.verify_pipeline"
QUEUE = "hotkey.control"


def celery_app(settings: Settings) -> Any:
    app = Celery("hotkey", broker=settings.broker_url.get_secret_value())
    app.conf.update(
        task_queues=(
            Queue(
                QUEUE,
                Exchange("hotkey", type="direct", durable=True),
                routing_key=QUEUE,
                durable=True,
            ),
        ),
        task_default_queue=QUEUE,
        task_default_exchange="hotkey",
        task_default_routing_key=QUEUE,
        task_create_missing_queues=False,
        task_serializer="json",
        accept_content=["json"],
        task_ignore_result=True,
        task_acks_late=True,
        task_reject_on_worker_lost=True,
        worker_prefetch_multiplier=1,
        task_publish_retry=False,
        broker_connection_timeout=5,
        broker_heartbeat=10,
        broker_connection_retry_on_startup=True,
        broker_transport_options={"confirm_publish": True},
        task_time_limit=20,
        worker_max_tasks_per_child=100,
        worker_enable_remote_control=False,
    )
    return app


def publish(app: Any, message: Dispatch) -> None:
    def reject_return(*args: Any, **kwargs: Any) -> None:
        raise RuntimeError("broker_unroutable")

    with app.connection_for_write() as connection:
        connection.ensure_connection(max_retries=0)
        producer = app.amqp.Producer(connection, on_return=reject_return)
        app.send_task(
            TASK_NAME,
            args=[message.model_dump(mode="json")],
            producer=producer,
            queue=QUEUE,
            retry=False,
            mandatory=True,
            delivery_mode=2,
            timeout=5,
            confirm_timeout=5,
        )


def dispatch_one(factory: sessionmaker[Session], app: Any) -> bool:
    # Lock only one bounded publish; failure rolls back sent_at, allowing replay.
    with factory.begin() as session:
        row = session.scalar(
            select(Outbox)
            .where(
                Outbox.sent_at.is_(None),
                Outbox.discarded_at.is_(None),
                Outbox.due_at <= utcnow(),
            )
            .order_by(Outbox.due_at, Outbox.id)
            .limit(1)
            .with_for_update(skip_locked=True)
        )
        if row is None:
            return False
        job = session.get(Job, row.job_id)
        if job is not None and job.epoch == row.epoch and job.status == "queued":
            publish(app, Dispatch(job_id=row.job_id, epoch=row.epoch))
            row.sent_at = utcnow()
        else:
            row.discarded_at = utcnow()
        return True
