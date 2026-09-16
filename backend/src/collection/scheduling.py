from datetime import UTC, datetime, timedelta
from hashlib import sha256

from sqlalchemy.orm import Session, sessionmaker

from collection.schemas import CollectionRunBatchInput
from collection.services import CollectionService
from core.errors import AppError
from monitors.services import MonitorService
from sources.services import SourceService


class CollectionScheduler:
    def __init__(
        self,
        factory: sessionmaker[Session],
        sources: SourceService | None = None,
        *,
        evidence_configured: bool = False,
    ):
        self.factory = factory
        self.sources = sources or SourceService()
        self.evidence_configured = evidence_configured

    @staticmethod
    def _slot(now: datetime, interval_minutes: int) -> tuple[datetime, datetime]:
        if now.tzinfo is None:
            raise ValueError("scheduler time must include a timezone")
        normalized = now.astimezone(UTC)
        seconds = interval_minutes * 60
        boundary = datetime.fromtimestamp(
            int(normalized.timestamp()) // seconds * seconds,
            tz=UTC,
        )
        return boundary - timedelta(seconds=seconds), boundary

    @staticmethod
    def _key(version_id: object, slot: datetime) -> str:
        canonical = f"{version_id}\0{slot.isoformat()}".encode()
        return "slot:" + sha256(canonical).hexdigest()

    def schedule_due(self, now: datetime) -> int:
        if not self.evidence_configured:
            return 0
        collection = CollectionService(
            self.factory,
            self.sources,
            evidence_configured=True,
        )
        created = 0
        for configuration in MonitorService(self.factory, self.sources).active_configurations():
            since, until = self._slot(now, configuration.schedule.interval_minutes)
            try:
                batch = collection.create_monitor_runs(
                    CollectionRunBatchInput(
                        monitor_id=configuration.monitor_id,
                        expected_version=configuration.version,
                        idempotency_key=self._key(
                            configuration.monitor_version_id,
                            until,
                        ),
                        trigger="scheduled",
                        ingestion_mode="live",
                        since=since,
                        until=until,
                        schedule_slot=until,
                    )
                )
            except AppError as error:
                if error.code in {
                    "monitor_not_active",
                    "version_conflict",
                    "source_not_eligible",
                    "request_budget_exhausted",
                }:
                    continue
                raise
            if not batch.replayed:
                created += len(batch.items)
        return created
