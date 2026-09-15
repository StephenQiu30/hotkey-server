import json
from datetime import UTC, datetime, timedelta
from hashlib import sha256

from sqlalchemy.orm import Session, sessionmaker

from collection.schemas import CollectionRunInput
from collection.services import CollectionService
from core.errors import AppError
from monitors.services import MonitorService
from sources.schemas import QueryPreviewInput
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
    def _key(version_id: object, source: str, query: str, slot: datetime) -> str:
        canonical = json.dumps(
            {
                "monitor_version_id": str(version_id),
                "source": source,
                "query": query,
                "slot": slot.isoformat(),
            },
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode()
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
            if self.sources.activation_issues(configuration.source_ids):
                continue
            since, until = self._slot(now, configuration.schedule.interval_minutes)
            preview = self.sources.preview(
                QueryPreviewInput(
                    query_spec=configuration.query_spec,
                    source_ids=configuration.source_ids,
                    since=since,
                    until=until,
                )
            )
            for source in preview.sources:
                for query in source.queries:
                    key = self._key(
                        configuration.monitor_version_id,
                        source.source,
                        query,
                        until,
                    )
                    if collection.has_scheduled_run(
                        configuration.monitor_version_id,
                        source.source,
                        source.operation,
                        query,
                        until,
                    ):
                        continue
                    try:
                        collection.create_run(
                            CollectionRunInput(
                                monitor_id=configuration.monitor_id,
                                expected_version=configuration.version,
                                source=source.source,
                                request_value=query,
                                since=since,
                                until=until,
                                idempotency_key=key,
                                policy_version=f"monitor-version-{configuration.version}",
                                retention_days=configuration.schedule.retention_days,
                                trigger="scheduled",
                                ingestion_mode="live",
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
                    created += 1
        return created
