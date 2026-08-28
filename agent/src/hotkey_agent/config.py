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
    model_base_url: str = ""
    model_api_key: str = ""
    model_name: str = ""
    model_version: str = ""
    model_timeout_seconds: int = 30
    model_max_response_bytes: int = 1_048_576
    model_max_output_tokens: int = 4_096

    @classmethod
    def from_env(cls) -> Settings:
        return cls(
            auth_token=os.getenv("HOTKEY_AGENT_AUTH_TOKEN", ""),
            runtime=os.getenv("HOTKEY_AGENT_RUNTIME", "deterministic"),
            max_request_bytes=_positive_int("HOTKEY_AGENT_MAX_REQUEST_BYTES", 262_144),
            max_concurrency=_positive_int("HOTKEY_AGENT_MAX_CONCURRENCY", 2),
            model_base_url=os.getenv("HOTKEY_AGENT_MODEL_BASE_URL", ""),
            model_api_key=os.getenv("HOTKEY_AGENT_MODEL_API_KEY", ""),
            model_name=os.getenv("HOTKEY_AGENT_MODEL_NAME", ""),
            model_version=os.getenv("HOTKEY_AGENT_MODEL_VERSION", ""),
            model_timeout_seconds=_bounded_int(
                "HOTKEY_AGENT_MODEL_TIMEOUT_SECONDS", 30, minimum=1, maximum=300
            ),
            model_max_response_bytes=_bounded_int(
                "HOTKEY_AGENT_MODEL_MAX_RESPONSE_BYTES",
                1_048_576,
                minimum=1,
                maximum=8_388_608,
            ),
            model_max_output_tokens=_bounded_int(
                "HOTKEY_AGENT_MODEL_MAX_OUTPUT_TOKENS", 4_096, minimum=1, maximum=32_768
            ),
        )

    @property
    def ready(self) -> bool:
        if len(self.auth_token.encode()) < 32:
            return False
        if self.runtime == "deterministic":
            return True
        return self.runtime == "openai_compatible" and self.model_ready

    @property
    def model_ready(self) -> bool:
        try:
            parsed = urlsplit(self.model_base_url)
            _ = parsed.port
        except ValueError:
            return False
        model_identity_valid = all(
            value == value.strip()
            and 1 <= len(value) <= limit
            and not any(character.isspace() for character in value)
            for value, limit in ((self.model_name, 128), (self.model_version, 32))
        )
        url_valid = (
            parsed.scheme == "https"
            and parsed.hostname is not None
            and parsed.username is None
            and parsed.password is None
            and parsed.query == ""
            and parsed.fragment == ""
        )
        api_key_valid = (
            self.model_api_key == self.model_api_key.strip()
            and "\r" not in self.model_api_key
            and "\n" not in self.model_api_key
            and len(self.model_api_key.encode()) >= 16
        )
        return url_valid and (
            api_key_valid
            and model_identity_valid
            and 1 <= self.model_timeout_seconds <= 300
            and 1 <= self.model_max_response_bytes <= 8_388_608
            and 1 <= self.model_max_output_tokens <= 32_768
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


def _bounded_int(name: str, default: int, *, minimum: int, maximum: int) -> int:
    raw = os.getenv(name)
    if raw is None:
        return default
    try:
        value = int(raw)
    except ValueError:
        return 0
    return value if minimum <= value <= maximum else 0
