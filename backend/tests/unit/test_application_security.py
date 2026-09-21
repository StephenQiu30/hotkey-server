import pytest
from pydantic import ValidationError

from core.config import Settings
from identity.schemas import IdentityCredentialsInput, IdentitySessionView


def test_identity_credentials_normalize_username_and_reject_weak_password() -> None:
    credentials = IdentityCredentialsInput(
        username="Owner.Name",
        password="a sufficiently long password",
    )
    assert credentials.username == "owner.name"

    with pytest.raises(ValidationError):
        IdentityCredentialsInput(username="owner", password="short")


def test_empty_bootstrap_value_is_unconfigured() -> None:
    settings = Settings(
        database_url="postgresql+psycopg://test:test@127.0.0.1/hotkey_test",
        bootstrap_token="",
    )
    assert settings.bootstrap_token is None


def test_session_response_contract_has_no_secret_fields() -> None:
    fields = IdentitySessionView.model_fields
    assert set(fields) == {"user", "expires_at"}
