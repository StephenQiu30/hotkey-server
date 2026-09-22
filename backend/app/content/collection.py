from __future__ import annotations

from collections.abc import Callable
from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy.orm import Session

from connections.services import require_web_connection_execution
from content.schemas import ContentRecordDetailView, PersistContentDocumentInput
from content.services import ContentService
from core.errors import ApplicationError
from evidence.services import LifecycleService
from jobs.execution import ExecutionLease, JobExecutionService, JobProgress
from jobs.schemas import JobStage
from sources.adapters.web_targets import normalize_web_url

type Clock = Callable[[], datetime]


class WebPageCommitService:
    """Commit a validated web document and its durable checkpoint atomically."""

    def __init__(
        self,
        session: Session,
        *,
        lease_seconds: int,
        clock: Clock | None = None,
    ) -> None:
        self._session = session
        self._lease_seconds = lease_seconds
        self._clock = clock or (lambda: datetime.now(UTC))

    def commit_document_page(
        self,
        *,
        owner_id: UUID,
        lease: ExecutionLease,
        command: PersistContentDocumentInput,
        sequence: int,
    ) -> tuple[ContentRecordDetailView, ExecutionLease]:
        fields = dict(command.admission.fields)
        request_url = fields.get("request_url")
        final_url = fields.get("final_url")
        if not isinstance(request_url, str) or not isinstance(final_url, str):
            raise ValueError("document URLs must be admitted as strings")

        execution = JobExecutionService(
            self._session,
            lease_seconds=self._lease_seconds,
            clock=self._clock,
        )
        self._session.rollback()
        with self._session.begin():
            execution.require_current_lease_in_transaction(lease)
            connection = require_web_connection_execution(
                self._session,
                owner_id=owner_id,
                connection_id=command.connection_id,
                connection_version=command.connection_version,
                target_url=request_url,
            )
            try:
                normalized_final_url = normalize_web_url(
                    final_url,
                    allowed_hosts=connection.allowed_hosts,
                )
            except ValueError as error:
                raise ApplicationError("source_target_not_allowed") from error
            LifecycleService(
                self._session,
                clock=self._clock,
            ).require_admission_in_transaction(
                owner_id=owner_id,
                admission=command.admission,
            )
            fields["request_url"] = connection.normalized_url
            fields["final_url"] = normalized_final_url
            normalized = command.model_copy(
                update={"admission": command.admission.model_copy(update={"fields": fields})}
            )
            content = ContentService(
                self._session,
                clock=self._clock,
            ).persist_document_in_transaction(
                owner_id=owner_id,
                command=normalized,
            )
            renewed = execution.save_checkpoint_in_transaction(
                lease,
                sequence=sequence,
                checkpoint={
                    "content_id": str(content.id),
                    "observation_id": str(content.latest_observation.id),
                },
                progress=JobProgress(stage=JobStage.SAVE, items_saved=1),
            )
        return content, renewed
