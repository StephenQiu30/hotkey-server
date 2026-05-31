import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
MIGRATION = ROOT / "migrations" / "000012_audit_logs.up.sql"
OPENAPI = ROOT / "docs" / "openapi.yaml"


class AdminContractTest(unittest.TestCase):
    def test_audit_log_migration_contract(self):
        sql = MIGRATION.read_text(encoding="utf-8")

        for required in [
            "CREATE TABLE IF NOT EXISTS audit_logs",
            "actor_id",
            "action",
            "resource_type",
            "resource_id",
            "result",
            "created_at",
        ]:
            self.assertIn(required, sql)

    def test_openapi_admin_observability_paths_are_documented(self):
        spec = OPENAPI.read_text(encoding="utf-8")

        for path in [
            "/api/v1/admin/config/status:",
            "/api/v1/admin/audit-logs:",
            "/api/v1/admin/jobs:",
            "/api/v1/admin/jobs/failed:",
            "/api/v1/admin/jobs/{jobID}:",
            "/api/v1/admin/jobs/{jobID}/retry:",
            "/api/v1/admin/daily-reports/rerun:",
        ]:
            self.assertIn(path, spec)

        for schema in [
            "AuditLog:",
            "ConfigStatus:",
            "Job:",
            "DailyReportRerunRequest:",
            "missing_config",
            "degraded",
        ]:
            self.assertIn(schema, spec)


if __name__ == "__main__":
    unittest.main()
