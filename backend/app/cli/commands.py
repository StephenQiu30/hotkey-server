from typing import Annotated

import typer
from minio import Minio
from redis import Redis

from core.config import get_settings
from core.errors import ApplicationError
from db.session import create_db_engine, create_session_factory
from evidence.adapters.cache import RedisCacheCleanup
from evidence.adapters.minio import MinioObjectCleanup
from evidence.schemas import CleanupTargetKind
from evidence.services import CleanupProcessor, LifecycleService
from identity.services import IdentityService

app = typer.Typer(no_args_is_help=True)
identity_app = typer.Typer(no_args_is_help=True)
lifecycle_app = typer.Typer(no_args_is_help=True)
app.add_typer(identity_app, name="identity")
app.add_typer(lifecycle_app, name="lifecycle")


@app.callback()
def main() -> None:
    """HotKey backend administration."""


@app.command()
def version() -> None:
    """Print the backend version."""
    typer.echo(get_settings().app_version)


@identity_app.command("reset-password")
def reset_password() -> None:
    """Reset the owner password and revoke every active session."""
    password = typer.prompt(
        "New password",
        hide_input=True,
        confirmation_prompt=True,
    )
    settings = get_settings()
    engine = create_db_engine(settings)
    session = create_session_factory(engine)()
    try:
        revoked_sessions = IdentityService(session, settings).reset_owner_password(password)
    except ApplicationError as error:
        typer.echo(f"Password reset failed: {error.code}", err=True)
        raise typer.Exit(code=1) from error
    finally:
        session.close()
        engine.dispose()
    typer.echo(f"Password reset complete; revoked sessions: {revoked_sessions}")


@lifecycle_app.command("cleanup-once")
def cleanup_once(
    limit: Annotated[int, typer.Option(min=1, max=1000)] = 100,
) -> None:
    """Register expired resources and process one bounded online-cleanup batch."""
    settings = get_settings()
    engine = create_db_engine(settings)
    sessions = create_session_factory(engine)
    redis = Redis.from_url(settings.redis_url)
    minio = Minio(
        settings.minio_endpoint,
        access_key=settings.minio_access_key,
        secret_key=settings.minio_secret_key,
        secure=settings.minio_secure,
    )
    try:
        with sessions() as session:
            expired = LifecycleService(session).expire_due(limit=limit)
        result = CleanupProcessor(
            sessions,
            handlers={
                CleanupTargetKind.REDIS_CACHE: RedisCacheCleanup(redis),
                CleanupTargetKind.MINIO_OBJECT: MinioObjectCleanup(
                    minio,
                    settings.minio_bucket,
                ),
            },
        ).process_due(limit=limit)
    finally:
        redis.close()
        engine.dispose()
    typer.echo(
        f"Lifecycle cleanup complete; expired: {len(expired)}, "
        f"succeeded: {result.succeeded}, failed: {result.failed}"
    )
    if result.failed:
        raise typer.Exit(code=1)
