import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
OPENAPI = ROOT / "docs" / "openapi.yaml"
MIGRATION = ROOT / "migrations" / "000009_reports_ai_summaries.up.sql"


class ReportContractTest(unittest.TestCase):
    def test_openapi_documents_report_endpoints_and_source_refs(self):
        text = OPENAPI.read_text(encoding="utf-8")
        for expected in [
            "/api/v1/reports:",
            "/api/v1/reports/{reportID}:",
            "DailyReport:",
            "DailyReportList:",
            "sourceRefs:",
            "promptVersion:",
            "failed_config",
        ]:
            self.assertIn(expected, text)

    def test_report_migration_schema_keeps_sources_and_status(self):
        text = MIGRATION.read_text(encoding="utf-8")
        for expected in [
            "CREATE TABLE IF NOT EXISTS ai_summaries",
            "CREATE TABLE IF NOT EXISTS daily_reports",
            "prompt_version text NOT NULL",
            "input_hotspot_ids_json jsonb NOT NULL",
            "source_refs_json jsonb NOT NULL",
            "date date NOT NULL",
        ]:
            self.assertIn(expected, text)
        self.assertNotIn("date text NOT NULL", text)


if __name__ == "__main__":
    unittest.main()
