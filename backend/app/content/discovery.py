from __future__ import annotations

from uuid import uuid5

from content.schemas import KeywordDiscoveryRunInput
from jobs.schemas import CollectionScanKind, JobAcceptanceInput, JobObservationContext
from sources.contracts import SourceCapability, SourceSort


def plan_keyword_discovery(run: KeywordDiscoveryRunInput) -> tuple[JobAcceptanceInput, ...]:
    """Freeze explicit source queries; acceptance waits for a registered, authorized handler."""
    observation = JobObservationContext(
        configuration_ref=run.configuration_ref,
        configuration_version=run.configuration_version,
        source_key=run.source_key,
        source_capability=SourceCapability.SEARCH,
    )
    jobs = []
    for sort, max_pages, max_requests in (
        (SourceSort.LATEST, run.latest_max_pages, run.latest_max_requests),
        (SourceSort.TOP, run.top_max_pages, run.top_max_requests),
    ):
        for index, query in enumerate((run.primary_query, *run.upstream_aliases)):
            jobs.append(
                JobAcceptanceInput(
                    operation_id=uuid5(run.run_id, f"{sort.value}:{index}"),
                    kind="keyword.search",
                    observation=observation,
                    scope={
                        "run_id": str(run.run_id),
                        "query": query,
                        "query_role": "primary" if index == 0 else "upstream_alias",
                        "sort_key": sort.value,
                        "starts_at": run.starts_at.isoformat(),
                        "ends_at": run.ends_at.isoformat(),
                        "page_size": run.page_size,
                        "max_pages": max_pages,
                        "max_requests": max_requests,
                        "scan_kind": CollectionScanKind.NEW_SCAN.value,
                    },
                )
            )
    return tuple(jobs)
