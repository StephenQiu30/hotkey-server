import typer

from core.config import get_settings

app = typer.Typer(no_args_is_help=True)


@app.callback()
def main() -> None:
    """HotKey backend administration."""


@app.command()
def version() -> None:
    """Print the backend version."""
    typer.echo(get_settings().app_version)
