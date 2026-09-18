from core.config import Settings


def test_settings_use_hotkey_environment_prefix(monkeypatch) -> None:
    monkeypatch.setenv("HOTKEY_APP_NAME", "HotKey Test")

    settings = Settings(database_url="postgresql+psycopg://test:test@127.0.0.1:5432/hotkey_test")

    assert settings.app_name == "HotKey Test"
