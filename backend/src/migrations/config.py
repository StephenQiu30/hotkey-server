from pathlib import Path

from alembic.config import Config


def migration_config(database_url: str) -> Config:
    config = Config()
    config.set_main_option("script_location", str(Path(__file__).resolve().parent))
    config.set_main_option("sqlalchemy.url", database_url.replace("%", "%%"))
    return config
