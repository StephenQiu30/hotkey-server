from pathlib import Path
from typing import Annotated
from uuid import UUID

import typer
from minio import Minio
from pydantic import ValidationError
from redis import Redis

from backups.adapters.minio import MinioObjectInventory, ObjectInventoryError
from backups.adapters.postgres import BackupToolError, PostgresDumpAdapter
from backups.services import BackupError, BackupService
from connections.schemas import (
    ConnectionEvidenceOutcome,
    ProbeEvidenceInput,
    SourceEntryPoint,
)
from connections.services import SourceCapabilityEvidenceService
from content.services import ContentObservationCleanup
from core.config import get_settings
from core.errors import ApplicationError
from db.session import create_db_engine, create_session_factory
from evidence.adapters.cache import RedisCacheCleanup
from evidence.adapters.minio import MinioObjectCleanup
from evidence.schemas import CleanupTargetKind
from evidence.services import CleanupProcessor, LifecycleService
from identity.services import IdentityService
from sources.contracts import SourceCapability, SourceStopReason

app = typer.Typer(no_args_is_help=True)
identity_app = typer.Typer(no_args_is_help=True)
lifecycle_app = typer.Typer(no_args_is_help=True)
backup_app = typer.Typer(no_args_is_help=True)
connections_app = typer.Typer(no_args_is_help=True)
app.add_typer(identity_app, name="identity")
app.add_typer(lifecycle_app, name="lifecycle")
app.add_typer(backup_app, name="backup")
app.add_typer(connections_app, name="connections")


@app.callback()
def main() -> None:
    """HotKey backend administration."""


@app.command()
def version() -> None:
    """Print the backend version."""
    typer.echo(get_settings().app_version)


@connections_app.command("record-probe")
def record_source_probe(
    owner_id: Annotated[UUID, typer.Option(help="Owner that controls the connection.")],
    connection_id: Annotated[UUID, typer.Option(help="Connection used by the probe.")],
    connection_version: Annotated[int, typer.Option(min=1, help="Version used by the probe.")],
    operation_id: Annotated[UUID, typer.Option(help="Idempotency key for this probe.")],
    capability: Annotated[SourceCapability, typer.Option(help="Capability that was checked.")],
    entry_point: Annotated[SourceEntryPoint, typer.Option(help="Entry point that was checked.")],
    outcome: Annotated[ConnectionEvidenceOutcome, typer.Option(help="Probe outcome.")],
    component_name: Annotated[str, typer.Option(help="Probe component name.")],
    component_version: Annotated[str, typer.Option(help="Probe component version.")],
    stop_reason: Annotated[
        SourceStopReason | None,
        typer.Option(help="Stable failure reason; required only when outcome is failed."),
    ] = None,
) -> None:
    """Record one explicitly executed source probe without exposing credentials."""
    try:
        command = ProbeEvidenceInput(
            operation_id=operation_id,
            connection_id=connection_id,
            connection_version=connection_version,
            capability=capability,
            entry_point=entry_point,
            outcome=outcome,
            stop_reason=stop_reason,
            component_name=component_name,
            component_version=component_version,
        )
    except ValidationError as error:
        typer.echo("Probe evidence failed: invalid_evidence", err=True)
        raise typer.Exit(code=1) from error

    settings = get_settings()
    engine = create_db_engine(settings)
    session = create_session_factory(engine)()
    try:
        evidence = SourceCapabilityEvidenceService(session).record_probe(
            owner_id=owner_id,
            command=command,
        )
    except ApplicationError as error:
        typer.echo(f"Probe evidence failed: {error.code}", err=True)
        raise typer.Exit(code=1) from error
    finally:
        session.close()
        engine.dispose()
    typer.echo(
        f"Probe evidence recorded: {evidence.id}; outcome: {evidence.outcome.value}; "
        f"connection version: {evidence.connection_version}"
    )


@backup_app.command("create-candidate")
def create_backup_candidate(
    destination: Annotated[
        Path,
        typer.Option(
            "--destination",
            exists=True,
            file_okay=False,
            dir_okay=True,
            writable=True,
            resolve_path=True,
            help="Existing directory that will receive one protected candidate bundle.",
        ),
    ],
) -> None:
    """Create a database archive and evidence-object inventory candidate."""
    settings = get_settings()
    engine = create_db_engine(settings)
    minio = Minio(
        settings.minio_endpoint,
        access_key=settings.minio_access_key,
        secret_key=settings.minio_secret_key,
        secure=settings.minio_secure,
    )
    try:
        result = BackupService(
            engine=engine,
            archive_writer=PostgresDumpAdapter(settings.database_url.get_secret_value()),
            object_inspector=MinioObjectInventory(minio, settings.minio_bucket),
            evidence_bucket=settings.minio_bucket,
            schema_path=Path(__file__).resolve().parents[2] / "database" / "schema.sql",
        ).create_candidate(destination)
    except (BackupError, BackupToolError, ObjectInventoryError) as error:
        typer.echo(f"Backup candidate failed: {error}", err=True)
        raise typer.Exit(code=1) from error
    finally:
        engine.dispose()
    missing = sum(item.state.value == "missing" for item in result.manifest.evidence_objects)
    typer.echo(
        f"Backup candidate created: {result.manifest.backup_id}; "
        f"path: {result.directory}; missing evidence objects: {missing}; "
        "restore verified: false"
    )


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
                CleanupTargetKind.POSTGRES_CONTENT_OBSERVATION: ContentObservationCleanup(sessions),
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
