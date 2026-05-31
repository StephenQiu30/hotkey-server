import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
MIGRATION = ROOT / "migrations" / "000002_users.up.sql"
OPENAPI = ROOT / "docs" / "openapi.yaml"


class AuthContractTest(unittest.TestCase):
    def test_users_and_refresh_tokens_schema_contract(self):
        sql = MIGRATION.read_text(encoding="utf-8")

        for required in [
            "CREATE TABLE IF NOT EXISTS users",
            "email",
            "password_hash",
            "role",
            "'user'",
            "'admin'",
            "status",
            "wechat_open_id",
            "CREATE TABLE IF NOT EXISTS refresh_tokens",
            "token_hash",
            "expires_at",
            "revoked_at",
        ]:
            self.assertIn(required, sql)

    def test_openapi_auth_paths_are_documented(self):
        spec = OPENAPI.read_text(encoding="utf-8")

        for path in [
            "/api/v1/auth/register:",
            "/api/v1/auth/login:",
            "/api/v1/auth/refresh:",
            "/api/v1/auth/logout:",
            "/api/v1/me:",
        ]:
            self.assertIn(path, spec)

        self.assertIn("bearerAuth:", spec)
        self.assertIn("Session:", spec)
        self.assertIn("UserResponse:", spec)
        self.assertIn("User:", spec)
        self.assertIn('$ref: "#/components/schemas/Session"', spec)
        self.assertIn('$ref: "#/components/schemas/UserResponse"', spec)
        self.assertIn("email_already_exists", spec)
        self.assertIn("invalid_credentials", spec)


if __name__ == "__main__":
    unittest.main()
