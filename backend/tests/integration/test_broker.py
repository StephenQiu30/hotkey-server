import os
import time

import pytest
from celery.contrib.testing.worker import start_worker
from kombu.exceptions import OperationalError
from sqlalchemy import func, select

from core.config import Settings
from jobs.contracts import Dispatch
from jobs.execution import claim, complete, enqueue
from jobs.models import Job, JobResult, Outbox
from worker.messaging import TASK_NAME, celery_app, dispatch_one, publish

pytestmark = pytest.mark.integration


def test_rabbitmq_delivery_and_duplicate_commit(database):
    broker = os.getenv("HOTKEY_TEST_BROKER_URL")
    if not broker:
        pytest.skip("RabbitMQ test endpoint not configured")
    from urllib.parse import urlsplit

    if urlsplit(broker).path != "/hotkey_test":
        pytest.fail("RabbitMQ integration requires isolated hotkey_test vhost")
    settings = Settings(database_url=os.environ["HOTKEY_TEST_DATABASE_URL"], broker_url=broker)
    app = celery_app(settings)

    @app.task(name=TASK_NAME)
    def consume(payload):
        lease = claim(database, Dispatch.model_validate(payload))
        if lease:
            complete(database, lease)

    with database.begin() as session:
        job_id = enqueue(session, "rabbitmq").id
    with start_worker(app, pool="solo", perform_ping_check=False):
        assert dispatch_one(database, app)
        publish(app, Dispatch(job_id=job_id, epoch=1))
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            with database() as session:
                if session.get(Job, job_id).status == "succeeded":
                    break
            time.sleep(0.1)
        else:
            pytest.fail("RabbitMQ consumer did not commit before deadline")
        # Drain the duplicate through the same consumer, not just a direct claim.
        with app.connection_for_write() as connection:
            queue = app.amqp.queues["hotkey.control"].bind(connection.default_channel)
            while queue.queue_declare(passive=True).message_count and time.monotonic() < deadline:
                time.sleep(0.1)
    with database() as session:
        assert session.scalar(select(func.count()).select_from(JobResult)) == 1
        assert session.get(Job, job_id).attempts == 1


def test_broker_failure_leaves_outbox_unsent(database):
    settings = Settings(
        database_url=os.environ["HOTKEY_TEST_DATABASE_URL"],
        broker_url="amqp://unused:unused@127.0.0.1:1/unused",
    )
    with database.begin() as session:
        enqueue(session, "broker-offline")
    with pytest.raises(OperationalError):
        dispatch_one(database, celery_app(settings))
    with database() as session:
        assert session.scalar(select(Outbox)).sent_at is None
        assert session.scalar(select(Job)).status == "queued"


def test_cancelled_intent_is_discarded_without_claiming_broker_confirmation(database):
    from jobs.execution import cancel

    with database.begin() as session:
        job_id = enqueue(session, "cancel-before-publish").id
    cancel(database, job_id)
    # None is intentional: any attempted broker access must fail this test.
    assert dispatch_one(database, None)
    assert not dispatch_one(database, None)
    with database() as session:
        row = session.scalar(select(Outbox))
        assert row.sent_at is None
        assert row.discarded_at is not None
