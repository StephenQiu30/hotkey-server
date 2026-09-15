from datetime import timedelta

from collection.schemas import PageCommitInput
from collection.services import CollectionService
from evidence.contracts import EvidenceStore
from jobs.contracts import Lease
from sources.contracts import CollectionPageFetcher
from sources.schemas import CollectionPageInput
from sources.services import SourceService


class CollectionExecutor:
    def __init__(
        self,
        service: CollectionService,
        sources: SourceService,
        fetcher: CollectionPageFetcher,
        store: EvidenceStore,
    ):
        self.service = service
        self.sources = sources
        self.fetcher = fetcher
        self.store = store

    def execute(self, lease: Lease) -> bool:
        if lease.kind != "collect_page":
            raise ValueError("collection executor requires a collect_page lease")
        run = self.service.claim_for_job(lease.job_id, lease.fencing_token)
        if run is None:
            return self.service.result_committed_for_job(lease.job_id)
        if self.sources.activation_issues([run.source], run.operation):
            return self.service.fail_run(run.run_id, run.fencing_token, "source_not_eligible")
        page = self.fetcher.fetch(
            CollectionPageInput(
                source=run.source,
                operation=run.operation,
                request_value=run.request_value,
                since=run.since,
                until=run.until,
            )
        )
        if page.payload is None:
            return self.service.fail_run(
                run.run_id,
                run.fencing_token,
                page.result.code or "source_request_failed",
            )
        result = page.result
        if result.cursor is not None:
            result = result.model_copy(
                update={"status": "partial", "code": "page_limit", "cursor": None}
            )
        self.service.commit_page(
            PageCommitInput(
                run_id=run.run_id,
                fencing_token=run.fencing_token,
                page_key=page.page_key,
                request_fingerprint=page.request_fingerprint,
                media_type=page.media_type,
                payload=page.payload,
                retention_until=result.observed_at + timedelta(days=run.retention_days),
                policy_version=run.policy_version,
                result=result,
            ),
            self.store,
        )
        return True
