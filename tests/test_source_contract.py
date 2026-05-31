import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
MIGRATION = ROOT / "migrations" / "000004_sources.up.sql"
OPENAPI = ROOT / "docs" / "openapi.yaml"


class SourceContractTest(unittest.TestCase):
    def test_sources_schema_contract(self):
        sql = MIGRATION.read_text(encoding="utf-8")

        for required in [
            "CREATE TABLE sources",
            "CREATE TABLE source_channel_links",
            "CREATE TABLE collection_runs",
            "source_channel_links",
            "collection_runs",
            "CHECK (type IN ('rss', 'public_page'))",
            "CHECK (status IN ('enabled', 'disabled'))",
            "compliance_note ~ E'\\\\S'",
            "error ~ E'\\\\S'",
            "compliance_note",
            "fetch_interval_min",
            "rate_limit_per_hour",
        ]:
            self.assertIn(required, sql)

        self.assertNotIn("CREATE TABLE IF NOT EXISTS", sql)
        self.assertNotIn("CREATE INDEX IF NOT EXISTS", sql)

    def test_openapi_source_paths_are_documented(self):
        spec = OPENAPI.read_text(encoding="utf-8")

        for path in [
            "/api/v1/admin/sources:",
            "/api/v1/admin/sources/{sourceID}:",
            "/api/v1/admin/sources/{sourceID}/status:",
            "/api/v1/admin/sources/{sourceID}/collection-runs:",
            "/api/v1/admin/sources/{sourceID}/test-fetch:",
        ]:
            self.assertIn(path, spec)

        for schema in [
            "Source:",
            "SourceRequest:",
            "SourceStatusRequest:",
            "CollectionRun:",
            "compliance_note_required",
        ]:
            self.assertIn(schema, spec)


if __name__ == "__main__":
    unittest.main()
