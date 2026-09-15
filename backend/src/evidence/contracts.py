from dataclasses import dataclass
from typing import Protocol


@dataclass(frozen=True)
class StoredObject:
    bucket: str
    key: str
    sha256: str
    size: int


class EvidenceStore(Protocol):
    bucket: str

    def put(self, key: str, payload: bytes, expected_sha256: str) -> StoredObject: ...
