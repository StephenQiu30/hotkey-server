from __future__ import annotations

import asyncio

from server.app.models.keyword import Keyword
from server.app.models.source import Source

from server.app.core.settings import settings

from server.app.services.providers import Candidate, SourceIngestionError as SourceIngestionError
from server.app.services.providers import build_provider
from server.app.services.providers.selector import mark_source_failure, mark_source_success


async def _fetch_candidates_async(
    source: Source,
    keyword: Keyword,
    query: str | None = None,
    *,
    record_health: bool = False,
    timeout_seconds: float = 8.0,
) -> list[Candidate]:
    provider = build_provider(source, keyword)
    try:
        candidates = await asyncio.wait_for(provider.fetch_hot_topics(query=query), timeout=timeout_seconds)
        if record_health:
            mark_source_success(source)
        return candidates
    except asyncio.TimeoutError as exc:
        if record_health:
            mark_source_failure(source, reason=f"source_fetch_timeout_{timeout_seconds}")
        raise SourceIngestionError(f"Fetch timeout for source {source.name} after {timeout_seconds}s") from exc
    except Exception as exc:  # noqa: BLE001
        if record_health:
            mark_source_failure(source, reason=f"fetch_failed: {type(exc).__name__}")
        raise SourceIngestionError(f"Failed to fetch candidates from {source.name}: {exc}") from exc


def fetch_candidates(
    source: Source,
    keyword: Keyword,
    query: str | None = None,
    *,
    record_health: bool = False,
    timeout_seconds: float | None = None,
) -> list[Candidate]:
    timeout = float(timeout_seconds) if timeout_seconds is not None else float(settings.source_timeout_seconds)
    try:
        return asyncio.run(
            _fetch_candidates_async(
                source=source,
                keyword=keyword,
                query=query,
                record_health=record_health,
                timeout_seconds=timeout,
            )
        )
    except RuntimeError as exc:
        # In tests and WSGI/legacy sync endpoints this path should not hit a running loop.
        raise SourceIngestionError(f"Failed to fetch candidates from {source.name}: {exc}") from exc
    except SourceIngestionError:
        raise


__all__ = ["Candidate", "SourceIngestionError", "fetch_candidates"]
