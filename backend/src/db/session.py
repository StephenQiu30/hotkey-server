from sqlalchemy import create_engine
from sqlalchemy.orm import Session, sessionmaker

from core.config import Settings


class Database:
    def __init__(self, settings: Settings):
        self.engine = create_engine(
            settings.database_url.get_secret_value(),
            pool_pre_ping=True,
            hide_parameters=True,
            pool_size=2,
            max_overflow=2,
            pool_timeout=5,
            connect_args={"connect_timeout": 3, "options": "-c statement_timeout=15000"},
        )
        self.sessions: sessionmaker[Session] = sessionmaker(self.engine, expire_on_commit=False)

    def close(self) -> None:
        self.engine.dispose()
