from __future__ import annotations

import os
from dataclasses import dataclass


@dataclass(frozen=True, slots=True)
class Settings:
    auth_token: str
    runtime: str
    max_request_bytes: int
    max_concurrency: int

    @classmethod
    def from_env(cls) -> Settings:
        return cls(
            auth_token=os.getenv("HOTKEY_AGENT_AUTH_TOKEN", ""),
            runtime=os.getenv("HOTKEY_AGENT_RUNTIME", "deterministic"),
            max_request_bytes=_positive_int("HOTKEY_AGENT_MAX_REQUEST_BYTES", 262_144),
            max_concurrency=_positive_int("HOTKEY_AGENT_MAX_CONCURRENCY", 2),
        )

    @property
    def ready(self) -> bool:
        return len(self.auth_token.encode()) >= 32 and self.runtime == "deterministic"


def _positive_int(name: str, default: int) -> int:
    raw = os.getenv(name)
    if raw is None:
        return default
    try:
        value = int(raw)
    except ValueError:
        return default
    return value if value > 0 else default
