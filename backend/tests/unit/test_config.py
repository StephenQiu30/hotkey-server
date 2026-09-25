import pytest
from pydantic import ValidationError

from core.config import Settings


def test_settings_use_hotkey_environment_prefix(monkeypatch) -> None:
    monkeypatch.setenv("HOTKEY_APP_NAME", "HotKey Test")

    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1:5432/hotkey_test")

    assert settings.app_name == "HotKey Test"


def test_source_credentials_are_server_only_and_source_allowlisted(monkeypatch) -> None:
    secret = "controlled-credential-not-a-real-secret"
    monkeypatch.setenv("HOTKEY_SOURCE_CREDENTIALS", '{"x":"' + secret + '"}')
    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")
    assert settings.source_credentials["x"].get_secret_value() == secret
    assert secret not in repr(settings)
    assert secret not in settings.model_dump_json()
    monkeypatch.setenv("HOTKEY_SOURCE_CREDENTIALS", '{"unknown":"' + secret + '"}')
    with pytest.raises(ValidationError):
        Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")


def test_firecrawl_is_disabled_and_bounded_by_default(monkeypatch) -> None:
    monkeypatch.setenv("HOTKEY_FIRECRAWL_ENABLED", "true")
    monkeypatch.setenv("HOTKEY_FIRECRAWL_BASE_URL", "http://127.0.0.1:3002/")
    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")
    assert settings.firecrawl_enabled
    assert settings.firecrawl_base_url == "http://127.0.0.1:3002"
    assert settings.firecrawl_timeout_seconds == 20
    assert settings.firecrawl_max_response_bytes == 2 * 1024 * 1024

    monkeypatch.setenv("HOTKEY_FIRECRAWL_BASE_URL", "http://user:secret@127.0.0.1:3002")
    with pytest.raises(ValidationError):
        Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")


def test_browser_runtime_is_disabled_and_uses_fixed_internal_endpoint(monkeypatch) -> None:
    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")
    assert not settings.browser_enabled
    assert settings.browser_ws_url.get_secret_value() == ""
    assert settings.browser_connect_timeout_seconds == 5

    monkeypatch.setenv("HOTKEY_BROWSER_ENABLED", "true")
    monkeypatch.setenv("HOTKEY_BROWSER_WS_URL", "ws://browser:3000/")
    with pytest.raises(ValidationError):
        Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")

    monkeypatch.setenv("HOTKEY_BROWSER_WS_URL", "ws://browser:3000/ws/" + "a" * 48)
    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")
    assert settings.browser_enabled
    assert settings.browser_ws_url.get_secret_value().endswith("a" * 48)
    assert "a" * 48 not in repr(settings)
    assert "a" * 48 not in settings.model_dump_json()

    monkeypatch.setenv("HOTKEY_BROWSER_WS_URL", "ws://user:password@browser:3000/")
    with pytest.raises(ValidationError):
        Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")


def test_browser_state_directory_is_optional(monkeypatch) -> None:
    monkeypatch.setenv("HOTKEY_BROWSER_STATE_DIR", "")
    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")
    assert settings.browser_state_dir is None


def test_browser_deadline_fits_job_lease_and_kafka_poll_window() -> None:
    settings = Settings(
        database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test",
        browser_enabled=True,
        browser_ws_url="ws://browser:3000/ws/" + "a" * 48,
    )

    assert settings.browser_execution_timeout_seconds == 45
    assert settings.job_process_execution_timeout_seconds("webpage.collect") == 65
    assert settings.job_lease_seconds == 75
    assert settings.kafka_max_poll_interval_seconds == 120


def test_browser_deadline_cannot_exceed_job_lease_budget() -> None:
    with pytest.raises(ValidationError, match="job lease"):
        Settings(
            database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test",
            browser_enabled=True,
            browser_ws_url="ws://browser:3000/ws/" + "a" * 48,
            job_lease_seconds=74,
        )


def test_job_lease_must_leave_kafka_poll_margin() -> None:
    with pytest.raises(ValidationError, match="Kafka max poll interval"):
        Settings(
            database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test",
            job_lease_seconds=60,
            kafka_max_poll_interval_seconds=64,
        )


def test_disabled_source_runtimes_do_not_reserve_external_deadline_budget() -> None:
    settings = Settings(
        database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test",
        firecrawl_enabled=False,
        browser_enabled=False,
        job_lease_seconds=15,
        kafka_max_poll_interval_seconds=30,
    )

    assert not settings.browser_enabled
    assert settings.job_process_execution_timeout_seconds("webpage.collect") == 5


def test_firecrawl_timeout_includes_process_and_finalization_margins() -> None:
    settings = Settings(
        database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test",
        firecrawl_enabled=True,
        job_lease_seconds=35,
    )

    assert settings.job_process_execution_timeout_seconds("webpage.collect") == 25
    with pytest.raises(ValidationError, match="job lease"):
        Settings(
            database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test",
            firecrawl_enabled=True,
            job_lease_seconds=34,
        )


@pytest.mark.parametrize(
    ("kind", "expected"),
    [
        ("keyword.search", 90),
        ("source.comments", 90),
        ("analysis.annotate", 600),
        ("report.daily", 600),
        ("report.weekly", 600),
        ("notification.send", 60),
        ("knowledge.export", 60),
    ],
)
def test_job_process_deadline_depends_on_job_kind(kind: str, expected: int) -> None:
    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")

    assert settings.job_process_execution_timeout_seconds(kind) == expected


def test_unknown_job_kind_has_no_implicit_process_deadline() -> None:
    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test")

    with pytest.raises(ValueError, match="unsupported job kind"):
        settings.job_process_execution_timeout_seconds("unknown.task")
