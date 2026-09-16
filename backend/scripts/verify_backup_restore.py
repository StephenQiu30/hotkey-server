"""Exercise PostgreSQL backup restore and withdrawal replay in disposable Compose."""

import json
import os
import re
import stat
import subprocess
import tempfile
from datetime import UTC, datetime, timedelta
from hashlib import sha256
from pathlib import Path
from uuid import UUID, uuid4

from sqlalchemy import create_engine, delete, func, select, text
from sqlalchemy.engine import URL, make_url
from sqlalchemy.orm import Session, sessionmaker

from audit.models import Audit
from collection.models import CollectionRun
from collection.schemas import CollectionRunInput
from collection.services import CollectionService
from contents.models import Content, ContentVersion, ContentWithdrawalRecord
from contents.schemas import ContentWithdrawalInput, ContentWithdrawalManifest
from db.health import SCHEMA_REVISION
from evidence.models import RawPage
from jobs.models import Attempt, Job, JobResult, Outbox
from knowledge.services import KnowledgeService
from monitors.models import Monitor
from monitors.schemas import MonitorInput, MonitorStateChange
from monitors.services import MonitorService
from sources.schemas import SourceName
from sources.services import SourceService


class AdmittedSources(SourceService):
    def activation_issues(
        self, source_ids: list[SourceName], operation: str = "search_posts"
    ) -> list[SourceName]:
        return []


def compose(*arguments: str, timeout: int = 30) -> str:
    return subprocess.check_output(
        ["docker", "compose", *arguments],
        text=True,
        timeout=timeout,
    ).strip()


def validate_target() -> tuple[str, URL, str]:
    project = os.getenv("COMPOSE_PROJECT_NAME", "")
    if not re.fullmatch(r"hotkey-[a-z0-9-]+", project) or project == "hotkey":
        raise RuntimeError("backup_poc_requires_disposable_compose_project")
    raw_url = os.getenv("HOTKEY_BACKUP_POC_DATABASE_URL")
    if not raw_url:
        raise RuntimeError("backup_poc_database_url_required")
    url = make_url(raw_url)
    if (
        url.drivername != "postgresql+psycopg"
        or url.database != "hotkey"
        or url.host not in {"127.0.0.1", "localhost"}
    ):
        raise RuntimeError("backup_poc_requires_loopback_hotkey_database")
    container = compose("ps", "-q", "postgres", timeout=10)
    if not container:
        raise RuntimeError("backup_poc_postgres_not_running")
    actual_project = subprocess.check_output(
        [
            "docker",
            "inspect",
            "--format",
            '{{ index .Config.Labels "com.docker.compose.project" }}',
            container,
        ],
        text=True,
        timeout=10,
    ).strip()
    if actual_project != project:
        raise RuntimeError("backup_poc_compose_project_mismatch")
    return project, url, container


def write_private_manifest(manifest: ContentWithdrawalManifest) -> Path:
    descriptor, name = tempfile.mkstemp(prefix="hotkey-withdrawals-", suffix=".json")
    path = Path(name)
    try:
        with os.fdopen(descriptor, "wb") as destination:
            destination.write(manifest.model_dump_json(indent=2).encode())
        if stat.S_IMODE(path.stat().st_mode) != 0o600:
            raise RuntimeError("backup_poc_manifest_not_private")
        return path
    except Exception:
        path.unlink(missing_ok=True)
        raise


def restored_state(
    factory: sessionmaker[Session],
    content_id: UUID,
    raw_page_id: UUID,
    external_id: str,
) -> dict[str, object]:
    with factory() as session:
        content = session.get(Content, content_id)
        raw_page = session.get(RawPage, raw_page_id)
        if content is None or raw_page is None:
            raise RuntimeError("backup_poc_seed_missing")
        records = session.scalar(
            select(func.count())
            .select_from(ContentWithdrawalRecord)
            .where(
                ContentWithdrawalRecord.source == "bilibili",
                ContentWithdrawalRecord.provider_namespace == "comment",
                ContentWithdrawalRecord.external_id == external_id,
            )
        )
        return {
            "content_visibility": content.visibility,
            "raw_page_object_state": raw_page.object_state,
            "withdrawal_records": records or 0,
        }


def main() -> None:
    _, source_url, _ = validate_target()
    suffix = uuid4().hex[:12]
    restore_database = f"hotkey_restore_{suffix}"
    dump_path = f"/tmp/hotkey-backup-{suffix}.dump"
    external_id = f"backup-restore-poc-comment-{suffix}"
    manifest_path: Path | None = None
    source_engine = create_engine(source_url)
    source_factory = sessionmaker(source_engine, expire_on_commit=False)
    restore_engine = None
    monitor_id: UUID | None = None
    run_id: UUID | None = None
    job_id: UUID | None = None
    content_id: UUID | None = None
    raw_page_id: UUID | None = None
    result: dict[str, object] = {}
    try:
        with source_engine.connect() as connection:
            revision = connection.scalar(text("SELECT version_num FROM alembic_version"))
        if revision != SCHEMA_REVISION:
            raise RuntimeError("backup_poc_schema_revision_mismatch")

        sources = AdmittedSources()
        monitors = MonitorService(source_factory, sources)
        draft = monitors.create_monitor(
            MonitorInput.model_validate(
                {
                    "title": f"backup-restore-poc-{suffix}",
                    "query_spec": {"include_any": ["恢复演练"]},
                    "source_ids": ["bilibili"],
                    "budget": {"daily_requests": 120, "content_purchase_cost": 0},
                }
            )
        )
        monitor_id = draft.id
        monitor = monitors.change_state(
            draft.id,
            MonitorStateChange(expected_version=draft.current_version),
            "active",
        )
        start = datetime(2026, 9, 16, tzinfo=UTC)
        run = CollectionService(source_factory, sources, evidence_configured=True).create_run(
            CollectionRunInput(
                monitor_id=monitor.id,
                expected_version=monitor.current_version,
                source="bilibili",
                request_value="恢复演练",
                since=start,
                until=start + timedelta(days=1),
                idempotency_key=f"backup-restore-poc-{suffix}",
                policy_version="backup-restore-poc-v1",
                retention_days=7,
                ingestion_mode="live",
            )
        )
        run_id = run.id
        job_id = run.job_id
        raw_page_id = uuid4()
        content_id = uuid4()
        synthetic_text = "备份恢复演练合成内容"
        with source_factory.begin() as session:
            session.add(
                RawPage(
                    id=raw_page_id,
                    run_id=run.id,
                    source="bilibili",
                    operation="list_comments",
                    request_fingerprint=sha256(suffix.encode()).hexdigest(),
                    bucket="synthetic-backup-poc",
                    object_key=f"raw/poc/{suffix}.json.gz",
                    payload_sha256=sha256(b"{}").hexdigest(),
                    object_sha256=sha256(b"{}").hexdigest(),
                    response_bytes=2,
                    object_bytes=2,
                    media_type="application/json",
                    observed_at=start,
                    retention_until=start + timedelta(days=7),
                    policy_version="backup-restore-poc-v1",
                    object_state="available",
                    cleanup_attempts=0,
                )
            )
            session.flush()
            session.add(
                Content(
                    id=content_id,
                    source="bilibili",
                    provider_namespace="comment",
                    external_id=external_id,
                    kind="comment",
                    canonical_url="https://example.test/backup-restore-poc",
                    author_ref="synthetic-author",
                    root_external_id="backup-restore-poc-root",
                    parent_external_id=None,
                    relation_status="resolved",
                    visibility="available",
                    first_seen_at=start,
                    last_seen_at=start,
                )
            )
            session.add(
                ContentVersion(
                    id=uuid4(),
                    content_id=content_id,
                    version=1,
                    text=synthetic_text,
                    text_sha256=sha256(synthetic_text.encode()).hexdigest(),
                    published_at=start,
                    observed_at=start,
                    raw_page_id=raw_page_id,
                )
            )

        pg_dump_version = compose("exec", "-T", "postgres", "pg_dump", "--version")
        compose(
            "exec",
            "-T",
            "postgres",
            "pg_dump",
            "-U",
            "hotkey",
            "--format=custom",
            "--file",
            dump_path,
            "hotkey",
            timeout=60,
        )
        dump_bytes = int(compose("exec", "-T", "postgres", "stat", "-c", "%s", dump_path))
        if dump_bytes <= 0:
            raise RuntimeError("backup_poc_dump_empty")

        knowledge = KnowledgeService(source_factory)
        knowledge.withdraw_content(content_id, ContentWithdrawalInput(reason="deleted"))
        manifest = knowledge.export_withdrawal_manifest()
        matching_entries = [entry for entry in manifest.entries if entry.external_id == external_id]
        if len(matching_entries) != 1:
            raise RuntimeError("backup_poc_manifest_entry_missing")
        isolated_manifest = manifest.model_copy(update={"entries": matching_entries})
        manifest_path = write_private_manifest(isolated_manifest)

        compose("exec", "-T", "postgres", "createdb", "-U", "hotkey", restore_database)
        compose(
            "exec",
            "-T",
            "postgres",
            "pg_restore",
            "-U",
            "hotkey",
            "--no-owner",
            "--no-privileges",
            "--dbname",
            restore_database,
            dump_path,
            timeout=60,
        )
        restore_url = source_url.set(database=restore_database)
        restore_engine = create_engine(restore_url)
        restore_factory = sessionmaker(restore_engine, expire_on_commit=False)
        before = restored_state(restore_factory, content_id, raw_page_id, external_id)
        expected_before = {
            "content_visibility": "available",
            "raw_page_object_state": "available",
            "withdrawal_records": 0,
        }
        if before != expected_before:
            raise RuntimeError("backup_poc_restore_baseline_mismatch")

        restored_knowledge = KnowledgeService(restore_factory)
        first = restored_knowledge.replay_withdrawal_manifest(isolated_manifest)
        after_first = restored_state(restore_factory, content_id, raw_page_id, external_id)
        second = restored_knowledge.replay_withdrawal_manifest(isolated_manifest)
        after_second = restored_state(restore_factory, content_id, raw_page_id, external_id)
        expected_after = {
            "content_visibility": "deleted",
            "raw_page_object_state": "delete_pending",
            "withdrawal_records": 1,
        }
        if after_first != expected_after or after_second != expected_after:
            raise RuntimeError("backup_poc_replay_mismatch")
        if (first.applied, first.missing, second.applied, second.missing) != (1, 0, 1, 0):
            raise RuntimeError("backup_poc_replay_result_mismatch")
        result = {
            "scope": "postgres_backup_restore_withdrawal_replay",
            "schema_revision": revision,
            "pg_dump_version": pg_dump_version,
            "dump_format": "custom",
            "dump_bytes": dump_bytes,
            "manifest_entries": 1,
            "manifest_mode": "0600",
            "before_replay": before,
            "after_first_replay": after_first,
            "after_second_replay": after_second,
            "replay": {
                "first_applied": first.applied,
                "first_missing": first.missing,
                "second_applied": second.applied,
                "second_missing": second.missing,
            },
        }
    finally:
        if restore_engine is not None:
            restore_engine.dispose()
        try:
            compose(
                "exec",
                "-T",
                "postgres",
                "dropdb",
                "-U",
                "hotkey",
                "--if-exists",
                "--force",
                restore_database,
            )
        finally:
            try:
                compose("exec", "-T", "postgres", "rm", "-f", dump_path)
            finally:
                if manifest_path is not None:
                    manifest_path.unlink(missing_ok=True)
                if any(
                    value is not None
                    for value in (content_id, raw_page_id, run_id, job_id, monitor_id)
                ):
                    with source_factory.begin() as session:
                        session.execute(
                            delete(ContentWithdrawalRecord).where(
                                ContentWithdrawalRecord.source == "bilibili",
                                ContentWithdrawalRecord.provider_namespace == "comment",
                                ContentWithdrawalRecord.external_id == external_id,
                            )
                        )
                        if content_id is not None:
                            session.execute(delete(Content).where(Content.id == content_id))
                        if raw_page_id is not None:
                            session.execute(delete(RawPage).where(RawPage.id == raw_page_id))
                        if run_id is not None:
                            session.execute(delete(CollectionRun).where(CollectionRun.id == run_id))
                        if job_id is not None:
                            session.execute(delete(Outbox).where(Outbox.job_id == job_id))
                            session.execute(delete(Attempt).where(Attempt.job_id == job_id))
                            session.execute(delete(JobResult).where(JobResult.job_id == job_id))
                            session.execute(delete(Job).where(Job.id == job_id))
                        if monitor_id is not None:
                            session.execute(delete(Monitor).where(Monitor.id == monitor_id))
                        for audited_id in (content_id, monitor_id):
                            if audited_id is not None:
                                session.execute(
                                    delete(Audit).where(Audit.target.like(f"{audited_id}%"))
                                )
                    with source_factory() as session:
                        remaining = sum(
                            int(session.get(model, identity) is not None)
                            for model, identity in (
                                (Content, content_id),
                                (RawPage, raw_page_id),
                                (CollectionRun, run_id),
                                (Job, job_id),
                                (Monitor, monitor_id),
                            )
                            if identity is not None
                        )
                        remaining += int(
                            session.scalar(
                                select(func.count())
                                .select_from(ContentWithdrawalRecord)
                                .where(ContentWithdrawalRecord.external_id == external_id)
                            )
                            or 0
                        )
                        for audited_id in (content_id, monitor_id):
                            if audited_id is not None:
                                remaining += int(
                                    session.scalar(
                                        select(func.count())
                                        .select_from(Audit)
                                        .where(Audit.target.like(f"{audited_id}%"))
                                    )
                                    or 0
                                )
                    if remaining:
                        raise RuntimeError("backup_poc_source_cleanup_failed")
                source_engine.dispose()
    restored_databases = compose(
        "exec",
        "-T",
        "postgres",
        "psql",
        "-U",
        "hotkey",
        "-d",
        "postgres",
        "-Atc",
        f"SELECT count(*) FROM pg_database WHERE datname='{restore_database}'",
    )
    if restored_databases != "0":
        raise RuntimeError("backup_poc_restore_database_cleanup_failed")
    compose("exec", "-T", "postgres", "test", "!", "-e", dump_path)
    if manifest_path is not None and manifest_path.exists():
        raise RuntimeError("backup_poc_manifest_cleanup_failed")
    result["cleanup"] = {
        "restore_database_removed": True,
        "dump_removed": True,
        "manifest_removed": True,
        "source_fixture_removed": True,
    }
    print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    main()
