from typing import Literal, Self

from pydantic import Field, SecretStr, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict
from sqlalchemy.engine import make_url


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_prefix="HOTKEY_", extra="ignore", hide_input_in_errors=True
    )
    environment: Literal["development", "production"] = "development"
    allowed_origins: list[str] = ["http://localhost:8010", "http://127.0.0.1:8010"]
    cookie_secure: bool = False
    database_url: SecretStr
    broker_url: SecretStr
    lease_seconds: int = Field(default=30, ge=5, le=300)
    recovery_seconds: int = Field(default=120, ge=30, le=3600)

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
        from urllib.parse import urlsplit

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
        return self
