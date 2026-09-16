from sqlalchemy import text
from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.orm import Session, sessionmaker

from core.schemas import HealthView

SCHEMA_REVISION = "0019_collection_retry_budget"


def readiness(factory: sessionmaker[Session]) -> HealthView:
    try:
        with factory() as session:
            revision = session.scalar(text("SELECT version_num FROM alembic_version"))
        if revision != SCHEMA_REVISION:
            return HealthView(status="not_ready", code="schema_mismatch")
    except SQLAlchemyError:
        return HealthView(status="not_ready", code="database_unavailable")
    return HealthView(status="ready", scope="database_schema")
