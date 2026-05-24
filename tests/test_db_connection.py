from __future__ import annotations

import os
import unittest
from unittest.mock import patch

from apps.api.app.core.settings import settings
from apps.api.app.db.connection import resolve_database_url
from apps.api.app.db import init_schema


class DbConnectionTests(unittest.TestCase):
    def test_resolve_database_url_prefers_local_sqlite_when_pytest_without_sqlite_url(self) -> None:
        original_url = settings.database_url
        with (
            patch.dict(os.environ, {"PYTEST_CURRENT_TEST": "test_foo"}, clear=False),
            patch.object(settings, "database_url", "postgresql+psycopg://postgres:postgres@postgres:5432/ai_hotspot_radar"),
        ):
            resolved_url = resolve_database_url()
            self.assertTrue(resolved_url.startswith("sqlite:///"))
            self.assertTrue(resolved_url.endswith("hotkey_test.sqlite3"))

        settings.database_url = original_url

    def test_resolve_database_url_keeps_sqlite_setting(self) -> None:
        original_url = settings.database_url
        with patch.object(
            settings,
            "database_url",
            "sqlite:////tmp/explicit.sqlite3",
        ):
            self.assertEqual(resolve_database_url(), "sqlite:////tmp/explicit.sqlite3")

        settings.database_url = original_url

    def test_initialize_database_uses_model_metadata_for_sqlite(self) -> None:
        with (
            patch("apps.api.app.db.init_schema.resolve_database_url", return_value="sqlite:////tmp/hotspot_test.sqlite3"),
            patch("apps.api.app.db.init_schema.create_schema_from_models_for_dev") as create_schema_from_models_for_dev,
        ):
            init_schema.initialize_database()

        create_schema_from_models_for_dev.assert_called_once_with("sqlite:////tmp/hotspot_test.sqlite3")
