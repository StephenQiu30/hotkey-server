from __future__ import annotations

import unittest
from unittest.mock import MagicMock, patch

from server.app.core.settings import settings
from server.app.db.connection import resolve_database_url
from server.app.db import init_schema


class DbConnectionTests(unittest.TestCase):
    def test_resolve_database_url_keeps_postgresql_setting(self) -> None:
        with patch.object(
            settings,
            "database_url",
            "postgresql+psycopg://postgres:postgres@postgres:5432/ai_hotspot_radar",
        ):
            self.assertEqual(resolve_database_url(), "postgresql+psycopg://postgres:postgres@postgres:5432/ai_hotspot_radar")

    def test_resolve_database_url_rejects_unsupported_scheme(self) -> None:
        with patch.object(
            settings,
            "database_url",
            "sqlite:////tmp/local.sqlite3",
        ):
            with self.assertRaises(RuntimeError):
                resolve_database_url()

    def test_initialize_database_uses_sql_script_for_postgresql(self) -> None:
        statements = ["CREATE TABLE test_table (id INT)", "CREATE INDEX idx_test_table_id ON test_table(id)"]
        mock_engine = MagicMock()
        mock_connection = MagicMock()
        mock_engine.begin.return_value.__enter__.return_value = mock_connection

        with (
            patch("server.app.db.init_schema.resolve_database_url", return_value="postgresql+psycopg://postgres:postgres@postgres:5432/ai_hotspot_radar"),
            patch("server.app.db.init_schema.create_engine", return_value=mock_engine),
            patch.object(
                init_schema,
                "SCHEMA_PATH",
                MagicMock(read_text=lambda *args, **kwargs: ";".join(statements)),
            ),
        ):
            init_schema.initialize_database()

        mock_connection.exec_driver_sql.assert_any_call("CREATE TABLE test_table (id INT)")
        mock_connection.exec_driver_sql.assert_any_call("CREATE INDEX idx_test_table_id ON test_table(id)")

    def test_initialize_database_raises_for_unsupported_scheme(self) -> None:
        with (
            patch("server.app.db.init_schema.resolve_database_url", return_value="sqlite:///:memory:"),
            patch("server.app.db.init_schema.create_engine"),
        ):
            with self.assertRaises(ValueError):
                init_schema.initialize_database()
