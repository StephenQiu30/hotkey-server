import pytest
from pydantic import ValidationError

from core.config import Settings


def test_rejects_other_database_and_broker():
    with pytest.raises(ValidationError):
        Settings(database_url="sqlite:///bad.db", broker_url="redis://localhost")


def test_settings_repr_hides_connection_credentials():
    settings = Settings(
        database_url="postgresql+psycopg://u:private@localhost/db",
        broker_url="amqp://u:private@localhost//",
    )
    assert "private" not in repr(settings)


def test_production_requires_https_and_secure_cookies():
    base = dict(
        database_url="postgresql+psycopg://u:p@localhost/db",
        broker_url="amqp://u:p@localhost/test",
        environment="production",
    )
    with pytest.raises(ValidationError):
        Settings(**base)
    with pytest.raises(ValidationError):
        Settings(**base, cookie_secure=True, allowed_origins=["https://example.com/path"])
    settings = Settings(**base, cookie_secure=True, allowed_origins=["https://example.com"])
    assert settings.cookie_secure
