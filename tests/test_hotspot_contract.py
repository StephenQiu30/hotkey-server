import pathlib
import sqlite3
import unittest

import yaml


ROOT = pathlib.Path(__file__).resolve().parents[1]
MIGRATION = ROOT / "migrations" / "000008_hotspot_scores.up.sql"
OPENAPI = ROOT / "docs" / "openapi.yaml"


class HotspotContractTest(unittest.TestCase):
    def test_hotspot_scores_migration_contract(self):
        sql = MIGRATION.read_text(encoding="utf-8")
        sqlite_sql = (
            sql.replace("timestamptz", "text")
            .replace("DEFAULT now()", "DEFAULT CURRENT_TIMESTAMP")
            .replace("double precision", "real")
            .replace("jsonb", "text")
            .replace("gen_random_uuid()::text", "'placeholder'")
            .replace("REFERENCES hotspot_clusters (id) ON DELETE CASCADE", "")
        )
        conn = sqlite3.connect(":memory:")
        try:
            conn.executescript(sqlite_sql)
            columns = {
                row[1]
                for row in conn.execute(
                    "PRAGMA table_info(hotspot_scores)"
                ).fetchall()
            }
            indexes = {
                row[1]
                for row in conn.execute(
                    "PRAGMA index_list(hotspot_scores)"
                ).fetchall()
            }
        finally:
            conn.close()

        self.assertGreaterEqual(
            columns,
            {
                "id",
                "cluster_id",
                "total_score",
                "source_count_score",
                "freshness_score",
                "relevance_score",
                "propagation_score",
                "quality_score",
                "explanation",
                "score_version",
                "created_at",
                "updated_at",
            },
        )
        self.assertGreaterEqual(
            indexes,
            {
                "idx_hotspot_scores_cluster_id",
                "idx_hotspot_scores_total_score",
            },
        )

    def test_openapi_hotspot_paths_are_documented(self):
        spec = yaml.safe_load(OPENAPI.read_text(encoding="utf-8"))
        paths = spec["paths"]

        expected = {
            "/api/v1/hotspots": {"get": {"200", "401"}},
            "/api/v1/hotspots/{hotspotID}": {"get": {"200", "401", "404"}},
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
            "HotspotScore",
            "HotspotDetail",
            "HotspotList",
        ]:
            self.assertIn(schema, schemas)


if __name__ == "__main__":
    unittest.main()
