from alembic import context
from sqlalchemy import create_engine, inspect
from sqlalchemy.pool import NullPool

from db.metadata import Base

config = context.config
url = config.get_main_option("sqlalchemy.url")
assert url is not None
engine = create_engine(url, poolclass=NullPool)
with engine.connect() as connection:
    tables = inspect(connection).get_table_names()
    if tables and "alembic_version" not in tables:
        raise RuntimeError("Refusing to initialize a nonempty unversioned database")
    connection.rollback()
    context.configure(connection=connection, target_metadata=Base.metadata)
    with context.begin_transaction():
        context.run_migrations()
engine.dispose()
