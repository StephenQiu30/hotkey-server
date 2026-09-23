import asyncio
import os
from pathlib import Path
from typing import Annotated
from uuid import UUID

import typer
from minio import Minio
from playwright.async_api import Error as PlaywrightError
from pydantic import ValidationError
from redis import Redis

from backups.adapters.minio import MinioObjectInventory, ObjectInventoryError
from backups.adapters.postgres import BackupToolError, PostgresDumpAdapter
from backups.restore import BackupRestoreError, BackupRestoreService
from backups.services import BackupError, BackupService
from connections.adapters.local_secrets import BrowserStateError, BrowserStateStore
from connections.schemas import (
    ConnectionEvidenceOutcome,
    ProbeEvidenceInput,
    SourceEntryPoint,
)
from connections.services import SourceCapabilityEvidenceService, SourceConnectionService
from content.services import ContentObservationCleanup
from core.config import get_settings
from core.errors import ApplicationError
from db.session import create_db_engine, create_session_factory
from evidence.adapters.cache import RedisCacheCleanup
from evidence.adapters.minio import MinioObjectCleanup
from evidence.schemas import CleanupTargetKind
from evidence.services import CleanupProcessor, LifecycleService
from identity.services import IdentityService
from sources.adapters.browser_runtime import BrowserRuntime, BrowserRuntimeDisabledError
from sources.adapters.firecrawl import FirecrawlAdapter
from sources.contracts import SourceCapability, SourceStopReason, WebPageRequest

app = typer.Typer(no_args_is_help=True)
identity_app = typer.Typer(no_args_is_help=True)
lifecycle_app = typer.Typer(no_args_is_help=True)
backup_app = typer.Typer(no_args_is_help=True)
connections_app = typer.Typer(no_args_is_help=True)
sources_app = typer.Typer(no_args_is_help=True)
app.add_typer(identity_app, name="identity")
app.add_typer(lifecycle_app, name="lifecycle")
app.add_typer(backup_app, name="backup")
app.add_typer(connections_app, name="connections")
app.add_typer(sources_app, name="sources")


@app.callback()
def main() -> None:
    """HotKey backend administration."""


@app.command()
def version() -> None:
    """Print the backend version."""
    typer.echo(get_settings().app_version)


@sources_app.command("probe-webpage")
def probe_webpage(
    url: Annotated[str, typer.Option(help="HTTP page to probe once.")],
    allowed_host: Annotated[
        str,
        typer.Option(help="Exact target host allowed for this probe."),
    ],
) -> None:
    """Run one explicit web-page probe without persisting target content."""
    settings = get_settings()
    try:
        request = WebPageRequest(
            url=url,
            timeout_seconds=settings.firecrawl_timeout_seconds,
        )
        adapter = FirecrawlAdapter(
            base_url=settings.firecrawl_base_url,
            enabled=settings.firecrawl_enabled,
            allowed_hosts=frozenset({allowed_host}),
            max_response_bytes=settings.firecrawl_max_response_bytes,
        )
    except (ValidationError, ValueError) as error:
        typer.echo(
            "Web page probe complete; status: failed; reason: invalid_request",
            err=True,
        )
        raise typer.Exit(code=1) from error

    with adapter:
        result = adapter.fetch_document(request)

    target_status = (
        "unknown" if result.target_status_code is None else str(result.target_status_code)
    )
    target_requests = (
        "unknown" if result.target_request_count is None else str(result.target_request_count)
    )
    if result.document is None:
        reason = (
            result.stop_reason.value
            if result.stop_reason is not None
            else SourceStopReason.PROTOCOL_ERROR.value
        )
        typer.echo(
            f"Web page probe complete; status: failed; reason: {reason}; "
            f"target status: {target_status}; "
            f"collector calls: {result.collector_call_count}; "
            f"target requests: {target_requests}",
            err=True,
        )
        raise typer.Exit(code=1)

    document = result.document
    typer.echo(
        "Web page probe complete; status: succeeded; "
        f"target status: {target_status}; characters: {len(document.text)}; "
        f"text scope: {document.text_scope}; extractor: {document.extractor_version}; "
        f"collector calls: {result.collector_call_count}; "
        f"target requests: {target_requests}"
    )


@sources_app.command("probe-browser")
def probe_browser() -> None:
    """Verify the managed browser connection without visiting a target website."""
    settings = get_settings()
    runtime = BrowserRuntime(
        ws_url=settings.browser_ws_url.get_secret_value(),
        enabled=settings.browser_enabled,
        connect_timeout_ms=settings.browser_connect_timeout_seconds * 1_000,
    )

    async def verify() -> None:
        async with runtime.context() as context:
            page = await context.new_page()
            await page.set_content('<main data-hotkey-browser-probe="ready"></main>')
            if await page.locator('[data-hotkey-browser-probe="ready"]').count() != 1:
                raise ValueError("browser probe failed")

    try:
        asyncio.run(verify())
    except (BrowserRuntimeDisabledError, PlaywrightError, TimeoutError, ValueError) as error:
        typer.echo("Browser probe complete; status: failed", err=True)
        raise typer.Exit(code=1) from error
    typer.echo("Browser probe complete; status: succeeded")


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


@connections_app.command("rotate-browser-state")
def rotate_browser_state(
    owner_id: Annotated[UUID, typer.Option(help="Owner of an existing browser connection.")],
    connection_id: Annotated[UUID, typer.Option(help="Existing browser connection to rotate.")],
    expected_version: Annotated[int, typer.Option(min=1, help="Current connection version.")],
    capture_file: Annotated[Path, typer.Option(help="Absolute private Playwright state file.")],
) -> None:
    """Activate a captured state on an existing browser-state connection."""
    settings = get_settings()
    if settings.browser_state_dir is None:
        typer.echo("Browser state update failed: state_directory_unconfigured", err=True)
        raise typer.Exit(code=1)
    try:
        store = BrowserStateStore(settings.browser_state_dir)
        state = store.read_capture(capture_file)
    except BrowserStateError as error:
        typer.echo("Browser state update failed: capture_invalid", err=True)
        raise typer.Exit(code=1) from error

    engine = create_db_engine(settings)
    session = create_session_factory(engine)()
    try:
        connection = SourceConnectionService(session).rotate_browser_state(
            owner_id=owner_id,
            connection_id=connection_id,
            expected_version=expected_version,
            state=state,
            store=store,
        )
    except ApplicationError as error:
        typer.echo(f"Browser state update failed: {error.code}", err=True)
        raise typer.Exit(code=1) from error
    finally:
        session.close()
        engine.dispose()
    typer.echo(f"Browser state updated: {connection.id}; version: {connection.version}")


@connections_app.command("disable-browser-state")
def disable_browser_state(
    owner_id: Annotated[UUID, typer.Option(help="Owner of an existing browser connection.")],
    connection_id: Annotated[UUID, typer.Option(help="Existing browser connection to stop.")],
    expected_version: Annotated[int, typer.Option(min=1, help="Current connection version.")],
) -> None:
    """Stop browser execution without opening a captured state file."""
    settings = get_settings()
    engine = create_db_engine(settings)
    session = create_session_factory(engine)()
    try:
        connection = SourceConnectionService(session).disable_browser_state(
            owner_id=owner_id,
            connection_id=connection_id,
            expected_version=expected_version,
        )
    except ApplicationError as error:
        typer.echo(f"Browser state disable failed: {error.code}", err=True)
        raise typer.Exit(code=1) from error
    finally:
        session.close()
        engine.dispose()
    typer.echo(f"Browser state disabled: {connection.id}; version: {connection.version}")


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


@backup_app.command("verify-restore")
def verify_backup_restore(
    candidate: Annotated[
        Path,
        typer.Option("--candidate", exists=True, file_okay=False, resolve_path=True),
    ],
    isolation_url_env: Annotated[
        str,
        typer.Option(
            "--isolation-url-env",
            help="Environment variable with an isolated maintenance database URL.",
        ),
    ],
) -> None:
    """Restore a candidate into a temporary database and verify its contents."""
    if not isolation_url_env.isidentifier() or isolation_url_env.upper() != isolation_url_env:
        typer.echo("Restore verification failed: invalid environment variable name", err=True)
        raise typer.Exit(code=1)
    isolation_url = os.getenv(isolation_url_env)
    if not isolation_url:
        typer.echo("Restore verification failed: isolation database URL is missing", err=True)
        raise typer.Exit(code=1)
    try:
        result = BackupRestoreService(
            source_database_url=get_settings().database_url.get_secret_value(),
            isolation_database_url=isolation_url,
            schema_path=Path(__file__).resolve().parents[2] / "database" / "schema.sql",
        ).verify(candidate)
    except BackupRestoreError as error:
        typer.echo(f"Restore verification failed: {error}", err=True)
        raise typer.Exit(code=1) from error
    typer.echo(
        f"Database restore verified: {result.backup_id}; tables: {result.table_count}; "
        f"duration seconds: {result.duration_seconds:.3f}; "
        "evidence objects: inventory only; complete backup verified: false"
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
