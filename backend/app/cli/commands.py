import typer

from core.config import get_settings
from core.errors import ApplicationError
from db.session import create_db_engine, create_session_factory
from identity.services import IdentityService

app = typer.Typer(no_args_is_help=True)
identity_app = typer.Typer(no_args_is_help=True)
app.add_typer(identity_app, name="identity")


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
