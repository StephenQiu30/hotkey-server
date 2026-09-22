from __future__ import annotations

from functools import lru_cache
from pathlib import Path
from typing import Literal

from pydantic import Field, SecretStr, field_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

_BACKEND_ROOT = Path(__file__).resolve().parents[2]


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

    job_lease_seconds: int = Field(default=60, ge=5, le=300)
    job_max_catchup_windows: int = Field(default=3, ge=1, le=100)

    minio_endpoint: str = "127.0.0.1:9000"
    minio_secure: bool = False
    minio_access_key: str = ""
    minio_secret_key: str = ""
    minio_bucket: str = "hotkey-evidence"

    @field_validator("bootstrap_token", mode="before")
    @classmethod
    def empty_bootstrap_token_is_unconfigured(cls, value: object) -> object:
        return None if value == "" else value


@lru_cache
def get_settings() -> Settings:
    return Settings()
