from __future__ import annotations

from collections.abc import Callable
from datetime import UTC, datetime, timedelta
from pathlib import Path
from uuid import UUID, uuid5

from sqlalchemy import select, text
from sqlalchemy.orm import Session, sessionmaker

from core.config import Settings
from jobs.execution import JobCompletion, JobExecutionFailure
from jobs.schemas import (
    JobAcceptanceInput,
    JobFailureCategory,
    JobMessage,
    JobObservationContext,
    JobStatus,
)
from jobs.services import JobService, load_job_execution_configuration
from knowledge.models import KnowledgeExport
from knowledge.obsidian import content_sha256, daily_relative_path, write_daily_note
from knowledge.schemas import DailyExportInput, KnowledgeExportResult, KnowledgeObjectType

EXPORT_OPERATION_NAMESPACE = UUID("bbd62d0a-49e8-4e41-984d-9ed7132412a8")
DAILY_OBJECT_NAMESPACE = UUID("263e04db-bb51-4b7d-9f3f-d356e4213c89")


def daily_object_id(*, owner_id: UUID, topic_id: UUID, window_start: datetime) -> UUID:
    return uuid5(DAILY_OBJECT_NAMESPACE, f"{owner_id}:{topic_id}:{window_start.isoformat()}")


def export_operation_id(*, report_id: UUID, version: int) -> UUID:
    return uuid5(EXPORT_OPERATION_NAMESPACE, f"{report_id}:{version}")


def _daily_input(row: dict[str, object]) -> DailyExportInput:
    data = row["data"]
    if not isinstance(data, dict) or not isinstance(data.get("topic_name"), str):
        raise ValueError("final report has no topic name")
    return DailyExportInput(
        owner_id=row["owner_id"],
        report_id=row["id"],
        topic_id=row["topic_id"],
        version=row["version"],
        topic_name=data["topic_name"],
        window_start=row["window_start"],
        generated_at=row["created_at"],
        generator=row["generator"],
        body_markdown=row["body_markdown"],
    )


class KnowledgeExportService:
    def __init__(self, session: Session, settings: Settings) -> None:
        self._session = session
        self._settings = settings

    def enqueue_due_in_transaction(self, *, now: datetime) -> int:
        if not self._settings.obsidian_enabled:
            return 0
        if not self._session.in_transaction():
            raise RuntimeError("knowledge scan requires the caller's transaction")
        if now.tzinfo is None:
            raise ValueError("knowledge scan time must be timezone-aware")
        rows = self._session.execute(
            text(
                """
                SELECT id, owner_id, topic_id, version, window_start, created_at,
                       generator, body_markdown, data
                FROM (
                    SELECT r.*, row_number() OVER (
                        PARTITION BY owner_id, topic_id, kind, window_start
                        ORDER BY version DESC
                    ) AS version_rank
                    FROM reports r WHERE kind = 'daily'
                ) latest
                WHERE version_rank = 1 AND status = 'final'
                ORDER BY owner_id, topic_id, window_start
                """
            )
        ).mappings()
        accepted = 0
        for row in rows:
            note = _daily_input(dict(row))
            object_id = daily_object_id(
                owner_id=note.owner_id,
                topic_id=note.topic_id,
                window_start=note.window_start,
            )
            prior = self._session.get(
                KnowledgeExport, (note.owner_id, KnowledgeObjectType.DAILY.value, object_id)
            )
            if prior is not None and prior.content_sha256 == content_sha256(note):
                continue
            JobService(self._session, clock=lambda: now).accept_in_transaction(
                owner_id=note.owner_id,
                command=JobAcceptanceInput(
                    operation_id=export_operation_id(
                        report_id=note.report_id, version=note.version
                    ),
                    kind="knowledge.export",
                    observation=JobObservationContext(
                        configuration_ref=f"report:{note.report_id}",
                        configuration_version=note.version,
                    ),
                    scope={"report_id": str(note.report_id), "version": note.version},
                ),
            )
            accepted += 1
        return accepted

    def export_daily_in_transaction(
        self, *, report_id: UUID, version: int
    ) -> KnowledgeExportResult | None:
        if not self._session.in_transaction():
            raise RuntimeError("knowledge export requires the caller's transaction")
        row = (
            self._session.execute(
                text(
                    """
                SELECT id, owner_id, topic_id, version, window_start, created_at,
                       generator, body_markdown, data, status
                FROM reports WHERE id = :report_id AND kind = 'daily'
                FOR UPDATE
                """
                ),
                {"report_id": report_id},
            )
            .mappings()
            .one_or_none()
        )
        if row is None or row["version"] != version or row["status"] != "final":
            raise ValueError("knowledge export report is absent or not final")
        note = _daily_input(dict(row))
        newer = self._session.execute(
            text(
                """
                SELECT 1 FROM reports
                WHERE owner_id = :owner_id AND topic_id = :topic_id AND kind = 'daily'
                  AND window_start = :window_start AND version > :version
                LIMIT 1
                """
            ),
            {
                "owner_id": note.owner_id,
                "topic_id": note.topic_id,
                "window_start": note.window_start,
                "version": note.version,
            },
        ).first()
        if newer is not None:
            return None
        object_id = daily_object_id(
            owner_id=note.owner_id, topic_id=note.topic_id, window_start=note.window_start
        )
        prior = self._session.get(
            KnowledgeExport, (note.owner_id, KnowledgeObjectType.DAILY.value, object_id)
        )
        path = (
            Path(prior.relative_path).relative_to(self._settings.obsidian_root)
            if prior is not None
            else daily_relative_path(note)
        )
        if prior is None:
            conflict = self._session.scalar(
                select(KnowledgeExport.object_id).where(
                    KnowledgeExport.owner_id == note.owner_id,
                    KnowledgeExport.relative_path
                    == (Path(self._settings.obsidian_root) / path).as_posix(),
                )
            )
            if conflict is not None:
                path = daily_relative_path(note, short_id=True)
        result = write_daily_note(
            self._settings.obsidian_vault_path,
            self._settings.obsidian_root,
            note,
            object_id,
            relative_path=path if prior is not None or path != daily_relative_path(note) else None,
        )
        if prior is None:
            prior = KnowledgeExport(
                owner_id=note.owner_id,
                object_type=KnowledgeObjectType.DAILY.value,
                object_id=object_id,
                relative_path=result.relative_path,
                content_sha256=result.content_sha256,
                exported_at=datetime.now(UTC),
            )
            self._session.add(prior)
        else:
            prior.relative_path = result.relative_path
            prior.content_sha256 = result.content_sha256
            prior.exported_at = datetime.now(UTC)
        self._session.flush()
        return result


class KnowledgeExportExecutor:
    def __init__(
        self,
        sessions: sessionmaker[Session],
        settings: Settings,
        *,
        clock: Callable[[], datetime] | None = None,
    ) -> None:
        self._sessions = sessions
        self._settings = settings
        self._clock = clock or (lambda: datetime.now(UTC))

    def execute(self, message: JobMessage) -> JobCompletion:
        if message.kind != "knowledge.export":
            raise ValueError("knowledge executor received another task kind")
        with self._sessions() as session, session.begin():
            configuration = load_job_execution_configuration(session, job_id=message.job_id)
            if (
                configuration is None
                or configuration.owner_id != message.owner_id
                or configuration.operation_id != message.operation_id
                or configuration.kind != message.kind
                or configuration.observation.configuration_ref != message.configuration_ref
                or configuration.observation.configuration_version != message.configuration_version
            ):
                raise ValueError("knowledge job configuration does not match the message")
            report_id = UUID(str(configuration.scope["report_id"]))
            version = configuration.scope["version"]
            if not isinstance(version, int) or isinstance(version, bool):
                raise ValueError("knowledge job version is invalid")
            if (
                message.configuration_ref != f"report:{report_id}"
                or message.configuration_version != version
                or message.operation_id != export_operation_id(report_id=report_id, version=version)
            ):
                raise ValueError("knowledge job does not match report version")
            try:
                KnowledgeExportService(session, self._settings).export_daily_in_transaction(
                    report_id=report_id, version=version
                )
            except OSError as error:
                now = self._clock().astimezone(UTC)
                retry_count = getattr(message, "retry_count", 0)
                raise JobExecutionFailure(
                    error_code="obsidian_vault_unavailable",
                    category=JobFailureCategory.TRANSIENT,
                    occurred_at=now,
                    next_action="检查 Obsidian vault 路径及写入权限后重试",
                    manual_retry_allowed=True,
                    retry_at=now + timedelta(seconds=30 * (2 ** min(retry_count, 2))),
                    max_attempts=3,
                ) from error
        return JobCompletion(status=JobStatus.SUCCEEDED)
