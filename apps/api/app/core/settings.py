from __future__ import annotations

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file="infra/env/.env", env_file_encoding="utf-8", extra="ignore")

    app_env: str = "development"
    app_name: str = "ai-hotspot-radar"
    app_timezone: str = "Asia/Shanghai"
    database_url: str = "postgresql+psycopg://root:change-me@localhost:5432/ai_hotspot_radar"
    database_init_retries: int = 10
    database_init_retry_seconds: float = 1.0
    openai_api_key: str | None = None
    openai_base_url: str = "https://api.openai.com/v1"
    openai_model: str | None = None
    deepseek_api_key: str | None = None
    deepseek_base_url: str = "https://api.deepseek.com/v1"
    deepseek_model: str | None = None
    gemini_api_key: str | None = None
    gemini_base_url: str = "https://generativelanguage.googleapis.com/v1beta/openai"
    gemini_model: str | None = None
    ai_provider: str = "openai"
    relevance_threshold: float = 50.0
    source_fetch_limit: int = 20
    x_api_bearer_token: str | None = None
    bing_search_api_key: str | None = None
    scheduler_enabled: bool = False
    check_interval_minutes: int = 60
    daily_report_enabled: bool = False
    daily_report_hour: int = 8
    weekly_report_enabled: bool = False
    weekly_report_weekday: int = 1
    weekly_report_hour: int = 8
    smtp_host: str | None = None
    smtp_port: int = 587
    smtp_username: str | None = None
    smtp_password: str | None = None
    smtp_from_email: str | None = None
    smtp_to_email: str | None = None
    smtp_use_tls: bool = True
    rss_access_token: str | None = None


settings = Settings()
