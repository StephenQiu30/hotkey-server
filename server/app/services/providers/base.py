from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass
from datetime import datetime
from typing import Any

from server.app.models.keyword import Keyword
from server.app.models.source import Source


class SourceIngestionError(RuntimeError):
    pass


@dataclass(slots=True)
class Candidate:
    title: str
    url: str
    source_id: int
    keyword_id: int | None
    author: str | None
    published_at: datetime | None
    snippet: str | None
    raw_payload: dict[str, Any]


class BaseProvider(ABC):
    """Base abstraction for source fetch providers."""

    source_type: str
    aliases: tuple[str, ...] = ()

    def __init__(self, source: Source, keyword: Keyword) -> None:
        self.source = source
        self.keyword = keyword

    @abstractmethod
    async def fetch_hot_topics(self, query: str | None = None) -> list[Candidate]:
        """Fetch normalized hotspot candidates for one keyword query."""


