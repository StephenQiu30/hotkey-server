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
