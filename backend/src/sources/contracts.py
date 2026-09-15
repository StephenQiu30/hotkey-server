import json
from collections.abc import Mapping
from hashlib import sha256
from typing import Literal, Protocol

from pydantic import BaseModel, ConfigDict, Field

from sources.schemas import SearchPageInput, SourceResult


def request_fingerprint(source: str, operation: str, parameters: Mapping[str, object]) -> str:
    canonical = json.dumps(
        {"source": source, "operation": operation, "parameters": parameters},
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    ).encode()
    return sha256(canonical).hexdigest()


class FetchedPage(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)
    result: SourceResult
    payload: bytes | None
    media_type: Literal["application/json", "text/html"]
    request_fingerprint: str = Field(pattern=r"^[a-f0-9]{64}$")
    page_key: str = Field(min_length=1, max_length=128, pattern=r"^[a-zA-Z0-9_.:-]+$")


class SearchPageFetcher(Protocol):
    def fetch(self, data: SearchPageInput) -> FetchedPage: ...
