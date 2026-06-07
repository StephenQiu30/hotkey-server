import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
MIGRATION = ROOT / "migrations" / "000013_authorizations.up.sql"
OPENAPI = ROOT / "docs" / "openapi.yaml"


class AuthorizationContractTest(unittest.TestCase):
    def test_authorizations_schema_contract(self):
        sql = MIGRATION.read_text(encoding="utf-8")

        for required in [
            "CREATE TABLE IF NOT EXISTS authorizations",
            "user_id",
            "platform",
            "platform_user_id",
            "access_token_enc",
            "status",
            "connected_at",
            "UNIQUE(user_id, platform)",
        ]:
            self.assertIn(required, sql)

    def test_openapi_authorization_paths_are_documented(self):
        spec = OPENAPI.read_text(encoding="utf-8")

        for path in [
            "/api/v1/authorizations:",
            "/api/v1/authorizations/connect:",
            "/api/v1/authorizations/{authorizationID}:",
            "/api/v1/authorizations/{authorizationID}/test:",
        ]:
            self.assertIn(path, spec)

        self.assertIn("AuthorizationResponse:", spec)
        self.assertIn("AuthorizationListResponse:", spec)
        self.assertIn("ConnectRequest:", spec)
        self.assertIn("Authorization:", spec)
        
        # Verify account deletion
        self.assertIn("/api/v1/me:", spec)
        me_section = spec.split("/api/v1/me:", 1)[1]
        next_api_path_idx = me_section.find("\n  /api/v1/")
        me_block = me_section if next_api_path_idx == -1 else me_section[:next_api_path_idx]
        self.assertIn("delete:", me_block)


if __name__ == "__main__":
    unittest.main()
