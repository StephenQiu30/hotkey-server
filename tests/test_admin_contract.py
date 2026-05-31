import pathlib
import sqlite3
import unittest

import yaml


ROOT = pathlib.Path(__file__).resolve().parents[1]
MIGRATION = ROOT / "migrations" / "000012_audit_logs.up.sql"
OPENAPI = ROOT / "docs" / "openapi.yaml"


class AdminContractTest(unittest.TestCase):
    def test_audit_log_migration_contract(self):
        sql = MIGRATION.read_text(encoding="utf-8")
        sqlite_sql = sql.replace("timestamptz", "text").replace(
            "DEFAULT now()", "DEFAULT CURRENT_TIMESTAMP"
        )
        conn = sqlite3.connect(":memory:")
        try:
            conn.executescript(sqlite_sql)
            columns = {
                row[1]
                for row in conn.execute("PRAGMA table_info(audit_logs)").fetchall()
            }
            indexes = {
                row[1]
                for row in conn.execute("PRAGMA index_list(audit_logs)").fetchall()
            }
        finally:
            conn.close()

        self.assertGreaterEqual(
            columns,
            {
                "id",
                "actor_id",
                "action",
                "resource_type",
                "resource_id",
                "result",
                "created_at",
            },
        )
        self.assertGreaterEqual(
            indexes,
            {
                "idx_audit_logs_actor_created_at",
                "idx_audit_logs_resource_created_at",
                "idx_audit_logs_created_at",
            },
        )
        for required in [
            "CHECK (action IN ('create', 'update', 'delete'))",
            "CHECK (result IN ('success', 'failure'))",
        ]:
            self.assertIn(required, sql)

    def test_openapi_admin_observability_paths_are_documented(self):
        spec = yaml.safe_load(OPENAPI.read_text(encoding="utf-8"))
        paths = spec["paths"]

        expected = {
            "/api/v1/admin/config/status": {"get": {"200", "403"}},
            "/api/v1/admin/audit-logs": {"get": {"200", "403"}},
            "/api/v1/admin/jobs": {"get": {"200", "403"}},
            "/api/v1/admin/jobs/failed": {"get": {"200", "403"}},
            "/api/v1/admin/jobs/{jobID}": {"get": {"200", "403", "404"}},
            "/api/v1/admin/jobs/{jobID}/retry": {"post": {"202", "403"}},
            "/api/v1/admin/daily-reports/rerun": {"post": {"202", "403"}},
        }
        for path, methods in expected.items():
            self.assertIn(path, paths)
            for method, response_codes in methods.items():
                self.assertIn(method, paths[path])
                self.assertGreaterEqual(
                    set(paths[path][method]["responses"]),
                    response_codes,
                )

        schemas = spec["components"]["schemas"]
        for schema in [
            "AuditLog",
            "ConfigStatus",
            "Job",
            "DailyReportRerunRequest",
        ]:
            self.assertIn(schema, schemas)

    def test_openapi_config_status_semantics_are_documented(self):
        spec = OPENAPI.read_text(encoding="utf-8")
        for required in [
            "missing_config",
            "degraded",
        ]:
            self.assertIn(required, spec)


if __name__ == "__main__":
    unittest.main()
