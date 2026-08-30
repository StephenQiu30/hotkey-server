from __future__ import annotations

import os
from dataclasses import dataclass
from urllib.parse import urlsplit


@dataclass(frozen=True, slots=True)
class Settings:
    auth_token: str
    runtime: str
    max_request_bytes: int
    max_concurrency: int
    previous_auth_tokens: tuple[str, ...] = ()
    codex_app_server_url: str = "ws://127.0.0.1:4500"

    @classmethod
    def from_env(cls) -> Settings:
        return cls(
            auth_token=os.getenv("HOTKEY_AGENT_AUTH_TOKEN", ""),
            previous_auth_tokens=_secret_list("HOTKEY_AGENT_PREVIOUS_AUTH_TOKENS"),
            runtime="codex_app_server",
            max_request_bytes=_positive_int("HOTKEY_AGENT_MAX_REQUEST_BYTES", 262_144),
            max_concurrency=_positive_int("HOTKEY_AGENT_MAX_CONCURRENCY", 2),
            codex_app_server_url=os.getenv(
                "HOTKEY_AGENT_CODEX_APP_SERVER_URL", "ws://127.0.0.1:4500"
            ),
        )

    @property
    def ready(self) -> bool:
        if len(self.auth_token.encode()) < 32:
            return False
        if any(
            len(token.encode()) < 32 or token == self.auth_token
            for token in self.previous_auth_tokens
        ) or len(set(self.previous_auth_tokens)) != len(self.previous_auth_tokens):
            return False
        if self.runtime == "deterministic":
            return True
        return self.runtime == "codex_app_server" and self.codex_app_server_ready

    @property
    def codex_app_server_ready(self) -> bool:
        try:
            parsed = urlsplit(self.codex_app_server_url)
            port = parsed.port
        except ValueError:
            return False
        return (
            parsed.scheme == "ws"
            and parsed.hostname in {"127.0.0.1", "::1", "localhost", "host.docker.internal"}
            and port is not None
            and parsed.username is None
            and parsed.password is None
            and parsed.path == ""
            and parsed.query == ""
            and parsed.fragment == ""
        )


def _positive_int(name: str, default: int) -> int:
    raw = os.getenv(name)
    if raw is None:
        return default
    try:
        value = int(raw)
    except ValueError:
        return default
    return value if value > 0 else default


def _secret_list(name: str) -> tuple[str, ...]:
    raw = os.getenv(name, "")
    if raw == "":
        return ()
    return tuple(item.strip() for item in raw.split(","))
