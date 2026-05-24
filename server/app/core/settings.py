from __future__ import annotations

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8", extra="ignore")

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
    github_client_id: str | None = None
    github_client_secret: str | None = None
    github_redirect_uri: str | None = None
    jwt_secret_key: str = "change-me-dev-secret-key"
    jwt_session_expire_days: int = 7
    oauth_state_ttl_seconds: int = 300
    web_base_url: str = "http://localhost:3000"
    ai_provider: str = "openai"
    ai_provider_error_strategy: str = "fallback"
    ai_fallback_provider: str = "fallback"
    relevance_threshold: float = 50.0
    source_fetch_limit: int = 20
    rate_limit_per_minute: int = 120
    x_api_bearer_token: str | None = None
    bing_search_api_key: str | None = None
    scheduler_enabled: bool = False
    check_interval_minutes: int = 60
    hotness_active_threshold: float = 70.0
    hotness_source_strength_default: float = 50.0
    hotness_min_freshness_hours: float = 72.0
    hotness_max_score: float = 100.0
    low_trust_penalty: float = 0.0
    source_failure_threshold: int = 3
    source_health_window_seconds: int = 300
    source_timeout_seconds: float = 8.0
    source_max_concurrency: int = 4
    ai_use_langgraph: bool = False
    ai_langgraph_timeout_seconds: int = 8
    ai_enhance_hotness_threshold: float = 85.0
    ai_enhance_risk_threshold: float = 60.0
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
    public_base_url: str = "http://localhost:8000"


settings = Settings()
