import pytest
from pydantic import ValidationError

from core.config import JOB_HARD_TIME_LIMIT_SECONDS, Settings
from worker.messaging import celery_app


def test_rejects_other_database_and_broker():
    with pytest.raises(ValidationError):
        Settings(database_url="sqlite:///bad.db", broker_url="redis://localhost")


def test_settings_repr_hides_connection_credentials():
    settings = Settings(
        database_url="postgresql+psycopg://u:private@localhost/db",
        broker_url="amqp://u:private@localhost//",
    )
    assert "private" not in repr(settings)


def test_job_lease_always_exceeds_worker_hard_time_limit():
    base = {
        "database_url": "postgresql+psycopg://u:p@localhost/db",
        "broker_url": "amqp://u:p@localhost/test",
    }
    settings = Settings(**base)
    assert settings.lease_seconds > JOB_HARD_TIME_LIMIT_SECONDS
    assert celery_app(settings).conf.task_time_limit == JOB_HARD_TIME_LIMIT_SECONDS
    with pytest.raises(ValidationError):
        Settings(**base, lease_seconds=JOB_HARD_TIME_LIMIT_SECONDS)


def test_minio_configuration_is_all_or_none_and_hides_credentials():
    base = {
        "database_url": "postgresql+psycopg://u:p@localhost/db",
        "broker_url": "amqp://u:p@localhost/test",
    }
    with pytest.raises(ValidationError):
        Settings(**base, s3_endpoint="minio.internal:9000")
    settings = Settings(
        **base,
        s3_endpoint="minio.internal:9000",
        s3_access_key="private-access",
        s3_secret_key="private-secret",
        s3_bucket="hotkey-evidence-test",
    )
    assert settings.s3_configured
    assert "private-access" not in repr(settings)
    assert "private-secret" not in repr(settings)
    with pytest.raises(ValidationError):
        Settings(
            **base,
            s3_endpoint="https://minio.internal:9000/path",
            s3_access_key="access",
            s3_secret_key="secret",
            s3_bucket="Invalid_Bucket",
        )
    for invalid_bucket in ("127.0.0.1", "invalid..bucket", "invalid.-bucket"):
        with pytest.raises(ValidationError):
            Settings(
                **base,
                s3_endpoint="minio.internal:9000",
                s3_access_key="access",
                s3_secret_key="secret",
                s3_bucket=invalid_bucket,
            )


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
