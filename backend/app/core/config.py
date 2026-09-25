from __future__ import annotations

import re
from functools import lru_cache
from pathlib import Path
from typing import Literal
from urllib.parse import urlsplit

from pydantic import Field, SecretStr, ValidationInfo, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

_BACKEND_ROOT = Path(__file__).resolve().parents[2]
BROWSER_EXECUTION_TIMEOUT_MAX_SECONDS = 45
BROWSER_CLOSE_TIMEOUT_SECONDS = 5
BROWSER_CLOSE_STEP_COUNT = 3
JOB_PROCESS_STARTUP_TIMEOUT_SECONDS = 3
JOB_PROCESS_HANDLER_SETUP_MARGIN_SECONDS = 5
JOB_PROCESS_TERMINATE_GRACE_SECONDS = 2
JOB_COMPLETION_MARGIN_SECONDS = 5
KAFKA_POLL_SAFETY_MARGIN_SECONDS = 5


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=_BACKEND_ROOT / ".env",
        env_prefix="HOTKEY_",
        extra="ignore",
    )

    app_name: str = "HotKey"
    app_version: str = "0.1.0"
    environment: Literal["development", "test", "staging", "production"] = "development"
    log_level: str = "INFO"

    database_url: SecretStr
    database_pool_size: int = Field(default=10, ge=1, le=100)
    database_max_overflow: int = Field(default=10, ge=0, le=100)
    database_pool_timeout_seconds: float = Field(default=10, gt=0, le=60)

    bootstrap_token: SecretStr | None = Field(default=None, min_length=32)
    session_ttl_seconds: int = Field(default=43_200, ge=900, le=86_400)
    source_credentials: dict[Literal["x", "douyin"], SecretStr] = Field(
        default_factory=dict, repr=False
    )

    @field_validator("source_credentials")
    @classmethod
    def validate_source_credentials(
        cls, value: dict[Literal["x", "douyin"], SecretStr]
    ) -> dict[Literal["x", "douyin"], SecretStr]:
        if any(not 32 <= len(secret.get_secret_value()) <= 65_536 for secret in value.values()):
            raise ValueError("source credentials must contain 32 to 65536 characters")
        return value

    redis_url: str = "redis://127.0.0.1:6379/0"
    kafka_bootstrap_servers: str = "127.0.0.1:9092"
    kafka_group_id: str = "hotkey-worker"
    kafka_delivery_timeout_seconds: int = Field(default=10, ge=1, le=60)
    kafka_max_poll_interval_seconds: int = Field(default=120, ge=30, le=900)

    job_lease_seconds: int = Field(default=75, ge=5, le=300)
    job_max_catchup_windows: int = Field(default=3, ge=1, le=100)

    firecrawl_enabled: bool = False
    firecrawl_base_url: str = "http://127.0.0.1:3002"
    firecrawl_timeout_seconds: int = Field(default=20, ge=1, le=20)
    firecrawl_max_response_bytes: int = Field(default=2 * 1024 * 1024, ge=1024, le=2 * 1024 * 1024)

    browser_enabled: bool = False
    browser_ws_url: SecretStr = SecretStr("")
    browser_connect_timeout_seconds: int = Field(default=5, ge=1, le=10)
    browser_execution_timeout_seconds: int = Field(
        default=BROWSER_EXECUTION_TIMEOUT_MAX_SECONDS,
        ge=1,
        le=BROWSER_EXECUTION_TIMEOUT_MAX_SECONDS,
    )
    browser_state_dir: Path | None = None

    minio_endpoint: str = "127.0.0.1:9000"
    minio_secure: bool = False
    minio_access_key: str = ""
    minio_secret_key: str = ""
    minio_bucket: str = "hotkey-evidence"

    def job_process_execution_timeout_seconds(self, kind: str) -> int:
        if kind in {"keyword.search", "source.comments"}:
            return 90
        if kind in {"analysis.annotate", "report.daily", "report.weekly"}:
            return 600
        if kind in {"notification.send", "knowledge.export"}:
            return 60
        if kind != "webpage.collect":
            raise ValueError(f"unsupported job kind: {kind}")
        execution_seconds = self.firecrawl_timeout_seconds if self.firecrawl_enabled else 0
        if self.browser_enabled:
            browser_execution_seconds = (
                self.browser_execution_timeout_seconds
                + BROWSER_CLOSE_TIMEOUT_SECONDS * BROWSER_CLOSE_STEP_COUNT
            )
            execution_seconds = max(execution_seconds, browser_execution_seconds)
        return execution_seconds + JOB_PROCESS_HANDLER_SETUP_MARGIN_SECONDS

    @field_validator("firecrawl_base_url")
    @classmethod
    def validate_firecrawl_base_url(cls, value: str) -> str:
        try:
            parsed = urlsplit(value)
            port = parsed.port
        except ValueError as error:
            raise ValueError("invalid Firecrawl base URL") from error
        if (
            parsed.scheme not in {"http", "https"}
            or parsed.hostname is None
            or parsed.username is not None
            or parsed.password is not None
            or parsed.path not in {"", "/"}
            or parsed.query
            or parsed.fragment
            or port is None
        ):
            raise ValueError("invalid Firecrawl base URL")
        return value.removesuffix("/")

    @field_validator("browser_ws_url")
    @classmethod
    def validate_browser_ws_url(cls, value: SecretStr, info: ValidationInfo) -> SecretStr:
        endpoint = value.get_secret_value()
        if not endpoint and not info.data.get("browser_enabled", False):
            return value
        try:
            parsed = urlsplit(endpoint)
            port = parsed.port
        except ValueError as error:
            raise ValueError("invalid browser WS URL") from error
        if (
            parsed.scheme not in {"ws", "wss"}
            or parsed.hostname is None
            or parsed.username is not None
            or parsed.password is not None
            or re.fullmatch(r"/ws/[0-9a-f]{48}", parsed.path) is None
            or parsed.query
            or parsed.fragment
            or port is None
        ):
            raise ValueError("invalid browser WS URL")
        return value

    @field_validator("bootstrap_token", mode="before")
    @classmethod
    def empty_bootstrap_token_is_unconfigured(cls, value: object) -> object:
        return None if value == "" else value

    @field_validator("browser_state_dir", mode="before")
    @classmethod
    def empty_browser_state_dir_is_unconfigured(cls, value: object) -> object:
        return None if value == "" else value

    @model_validator(mode="after")
    def validate_execution_deadlines(self) -> Settings:
        required_lease_seconds = (
            JOB_PROCESS_STARTUP_TIMEOUT_SECONDS
            + self.job_process_execution_timeout_seconds("webpage.collect")
            + JOB_PROCESS_TERMINATE_GRACE_SECONDS
            + JOB_COMPLETION_MARGIN_SECONDS
        )
        if self.job_lease_seconds < required_lease_seconds:
            raise ValueError(
                "job lease must include process startup, execution, termination, and completion"
            )
        if self.kafka_max_poll_interval_seconds < (
            self.job_lease_seconds + KAFKA_POLL_SAFETY_MARGIN_SECONDS
        ):
            raise ValueError("Kafka max poll interval must leave margin after the job lease")
        return self

    ai_model: str = "gpt-5.6-luna"
    ai_command: str = "codex app-server"
    ai_timeout_seconds: int = Field(default=300, ge=1, le=900)
    ai_effort: str = "low"


@lru_cache
def get_settings() -> Settings:
    return Settings()
