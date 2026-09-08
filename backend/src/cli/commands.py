import argparse
import json
import logging
import signal
import threading
from uuid import UUID

from alembic import command
from sqlalchemy import select

from core.config import Settings
from db.session import Database
from jobs.execution import cancel, enqueue, reconcile
from jobs.models import Job
from migrations.config import migration_config
from worker.messaging import celery_app, dispatch_one


def main() -> None:
    parser = argparse.ArgumentParser(description="HotKey Python foundation")
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("migrate")
    sub.add_parser("dispatch")
    sub.add_parser("reconcile")
    sub.add_parser("scheduler")
    sub.add_parser("enqueue").add_argument("key")
    sub.add_parser("show").add_argument("job_id", type=UUID)
    sub.add_parser("cancel").add_argument("job_id", type=UUID)
    owner = sub.add_parser("owner-init")
    owner.add_argument("username")
    owner.add_argument("--password-stdin", action="store_true")
    args = parser.parse_args()
    settings = Settings()
    if args.command == "migrate":
        command.upgrade(migration_config(settings.database_url.get_secret_value()), "head")
        return
    database = Database(settings)
    factory = database.sessions
    try:
        if args.command == "owner-init":
            import getpass
            import sys

            from identity.services import IdentityService

            password = (
                sys.stdin.readline().rstrip("\n") if args.password_stdin else getpass.getpass()
            )
            IdentityService(factory).bootstrap(args.username, password)
            print("Owner initialized")
        elif args.command == "enqueue":
            with factory.begin() as session:
                job_id = enqueue(session, args.key).id
            print(json.dumps({"job_id": str(job_id)}))
        elif args.command == "show":
            with factory() as session:
                job = session.scalar(select(Job).where(Job.id == args.job_id))
                print(
                    json.dumps(
                        None
                        if job is None
                        else {
                            "job_id": str(job.id),
                            "status": job.status,
                            "epoch": job.epoch,
                            "attempts": job.attempts,
                        }
                    )
                )
        elif args.command == "cancel":
            print(json.dumps({"cancelled": cancel(factory, args.job_id)}))
        elif args.command == "reconcile":
            print(json.dumps({"recovered": reconcile(factory, settings.recovery_seconds)}))
        elif args.command == "dispatch":
            print(json.dumps({"dispatched": dispatch_one(factory, celery_app(settings))}))
        else:
            stopped = threading.Event()
            signal.signal(signal.SIGTERM, lambda *_: stopped.set())
            signal.signal(signal.SIGINT, lambda *_: stopped.set())
            app = celery_app(settings)
            while not stopped.is_set():
                try:
                    reconcile(factory, settings.recovery_seconds)
                    for _ in range(100):
                        if stopped.is_set() or not dispatch_one(factory, app):
                            break
                except Exception as exc:
                    # Connection exceptions can contain credentials; log only the class.
                    logging.error("scheduler_iteration_failed type=%s", type(exc).__name__)
                stopped.wait(1)
    finally:
        database.close()
