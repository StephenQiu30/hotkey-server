from pathlib import Path


def test_single_runtime_stack():
    root = Path(__file__).resolve().parents[3]
    assert (root / "backend").is_dir()
    assert not (root / "server").exists()
    assert not (root / "agent").exists()
    assert "python-api" not in (root / "docker-compose.yml").read_text()
    assert (root / "docs/openapi/openapi.json").exists()


def test_migrations_are_available_in_application():
    from alembic.script import ScriptDirectory

    from db.health import SCHEMA_REVISION
    from migrations.config import migration_config

    config = migration_config("postgresql+psycopg://u:p@localhost/test")
    assert (
        Path(config.get_main_option("script_location"))
        == Path(__file__).resolve().parents[2] / "src" / "migrations"
    )
    assert ScriptDirectory.from_config(config).get_current_head() == SCHEMA_REVISION
