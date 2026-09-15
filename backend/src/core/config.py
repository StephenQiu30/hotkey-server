import ipaddress
import re
from typing import Literal, Self
from urllib.parse import urlsplit

from pydantic import Field, SecretStr, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict
from sqlalchemy.engine import make_url

JOB_HARD_TIME_LIMIT_SECONDS = 60


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_prefix="HOTKEY_", extra="ignore", hide_input_in_errors=True
    )
    environment: Literal["development", "production"] = "development"
    allowed_origins: list[str] = ["http://localhost:8010", "http://127.0.0.1:8010"]
    cookie_secure: bool = False
    database_url: SecretStr
    broker_url: SecretStr
    lease_seconds: int = Field(default=90, gt=JOB_HARD_TIME_LIMIT_SECONDS, le=300)
    recovery_seconds: int = Field(default=120, ge=30, le=3600)
    s3_endpoint: str | None = None
    s3_access_key: SecretStr | None = None
    s3_secret_key: SecretStr | None = None
    s3_bucket: str | None = None
    s3_secure: bool = True
    source_rights_allowed: list[str] = Field(default_factory=list, max_length=24)
    source_pipelines_connected: list[str] = Field(default_factory=list, max_length=24)
    embedding_base_url: str | None = None
    embedding_model: Literal["qwen3-embedding:latest"] = "qwen3-embedding:latest"
    embedding_model_digest: str | None = Field(default=None, pattern=r"^[a-f0-9]{64}$")
    embedding_dimensions: int = 1024
    embedding_timeout_seconds: float = Field(default=15.0, ge=1.0, le=60.0)

    @field_validator("source_rights_allowed", "source_pipelines_connected")
    @classmethod
    def validate_source_operations(cls, values: list[str]) -> list[str]:
        pattern = r"^[a-z][a-z0-9_]{0,31}\.[a-z][a-z0-9_]{0,31}$"
        if any(not re.fullmatch(pattern, value) for value in values):
            raise ValueError("Source admission values must use source.operation")
        return list(dict.fromkeys(values))

    @field_validator("s3_endpoint", "s3_access_key", "s3_secret_key", "s3_bucket", mode="before")
    @classmethod
    def empty_s3_value_is_absent(cls, value: object) -> object:
        if value == "" or isinstance(value, SecretStr) and not value.get_secret_value():
            return None
        return value

    @field_validator("embedding_base_url", "embedding_model_digest", mode="before")
    @classmethod
    def empty_embedding_origin_is_absent(cls, value: object) -> object:
        return None if value == "" else value

    @field_validator("embedding_base_url")
    @classmethod
    def validate_embedding_origin(cls, value: str | None) -> str | None:
        if value is None:
            return None
        parsed = urlsplit(value)
        try:
            _ = parsed.port
        except ValueError as error:
            raise ValueError("Embedding origin contains an invalid port") from error
        if (
            parsed.scheme not in {"http", "https"}
            or not parsed.hostname
            or parsed.username
            or parsed.password
            or parsed.path not in {"", "/"}
            or parsed.query
            or parsed.fragment
        ):
            raise ValueError("Embedding endpoint must be an exact HTTP(S) origin")
        return value.rstrip("/")

    @field_validator("embedding_dimensions")
    @classmethod
    def validate_embedding_dimensions(cls, value: int) -> int:
        if value != 1024:
            raise ValueError("Embedding dimensions must be 1024")
        return value

    @field_validator("s3_endpoint")
    @classmethod
    def validate_s3_endpoint(cls, value: str | None) -> str | None:
        if value is None:
            return None
        if "://" in value:
            raise ValueError("S3 endpoint must be host[:port] without a URL scheme")
        try:
            parsed = urlsplit("//" + value)
            _ = parsed.port
        except ValueError as error:
            raise ValueError("S3 endpoint contains an invalid port") from error
        if (
            not parsed.hostname
            or parsed.username
            or parsed.password
            or parsed.path
            or parsed.query
            or parsed.fragment
        ):
            raise ValueError("S3 endpoint must be host[:port] without credentials or a path")
        return value

    @field_validator("s3_bucket")
    @classmethod
    def validate_s3_bucket(cls, value: str | None) -> str | None:
        if value is None:
            return None
        valid = bool(re.fullmatch(r"[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]", value))
        valid = valid and not any(part in value for part in ("..", ".-", "-."))
        try:
            ipaddress.ip_address(value)
        except ValueError:
            pass
        else:
            valid = False
        if not valid:
            raise ValueError("S3 bucket must be a valid 3-63 character DNS-style name")
        return value

    @field_validator("database_url")
    @classmethod
    def postgres_only(cls, value: SecretStr) -> SecretStr:
        try:
            valid = make_url(value.get_secret_value()).drivername == "postgresql+psycopg"
        except Exception:
            valid = False
        if not valid:
            raise ValueError("SQLAlchemy postgresql+psycopg is required")
        return value

    @field_validator("broker_url")
    @classmethod
    def rabbitmq_only(cls, value: SecretStr) -> SecretStr:
        from urllib.parse import urlsplit

        try:
            parsed = urlsplit(value.get_secret_value())
            valid = parsed.scheme in {"amqp", "amqps"} and bool(parsed.hostname)
        except ValueError:
            valid = False
        if not valid:
            raise ValueError("RabbitMQ AMQP endpoint is required")
        return value

    @model_validator(mode="after")
    def validate_browser_boundary(self) -> Self:
        if not self.allowed_origins:
            raise ValueError("At least one exact browser origin is required")
        for origin in self.allowed_origins:
            parsed = urlsplit(origin)
            if (
                parsed.scheme not in {"http", "https"}
                or not parsed.hostname
                or parsed.path
                or parsed.query
                or parsed.fragment
                or parsed.username
                or "*" in origin
            ):
                raise ValueError("Origins must be exact HTTP(S) origins without paths")
            if self.environment == "production" and parsed.scheme != "https":
                raise ValueError("Production origins must use HTTPS")
        if self.environment == "production" and not self.cookie_secure:
            raise ValueError("Production requires secure cookies")
        s3_values = (
            self.s3_endpoint,
            self.s3_access_key,
            self.s3_secret_key,
            self.s3_bucket,
        )
        if any(value is not None for value in s3_values) and any(
            value is None for value in s3_values
        ):
            raise ValueError("S3 endpoint, access key, secret key and bucket are required together")
        if self.source_pipelines_connected and not self.s3_configured:
            raise ValueError("Connected source pipelines require a configured evidence store")
        if (self.embedding_base_url is None) != (self.embedding_model_digest is None):
            raise ValueError("Embedding origin and model digest are required together")
        return self

    @property
    def s3_configured(self) -> bool:
        return self.s3_endpoint is not None

    @property
    def embedding_configured(self) -> bool:
        return self.embedding_base_url is not None and self.embedding_model_digest is not None
