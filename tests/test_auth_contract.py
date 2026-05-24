from __future__ import annotations

import unittest
from datetime import datetime, timezone
from types import SimpleNamespace

from apps.api.app.core import security
from apps.api.app.core.security import issue_session_token, verify_signed_token
from apps.api.app.main import create_app
from apps.api.app.models.user import User
from apps.api.app.schemas.auth import UserRead


class AuthContractTests(unittest.TestCase):
    def test_openapi_exposes_email_password_miniapp_and_refresh_contracts(self) -> None:
        openapi = create_app().openapi()

        self.assertIn("/api/auth/register", openapi["paths"])
        self.assertIn("/api/auth/login", openapi["paths"])
        self.assertIn("/api/auth/miniapp/login", openapi["paths"])
        self.assertIn("/api/auth/token/refresh", openapi["paths"])

        schemas = openapi["components"]["schemas"]
        self.assertIn("EmailRegisterRequest", schemas)
        self.assertIn("EmailLoginRequest", schemas)
        self.assertIn("MiniappLoginRequest", schemas)
        self.assertIn("TokenRefreshResponse", schemas)

    def test_password_hashing_contract_does_not_store_plain_text(self) -> None:
        self.assertTrue(hasattr(security, "hash_password"))
        self.assertTrue(hasattr(security, "verify_password"))

        password_hash = security.hash_password("Correct Horse Battery Staple")

        self.assertNotEqual(password_hash, "Correct Horse Battery Staple")
        self.assertTrue(security.verify_password("Correct Horse Battery Staple", password_hash))
        self.assertFalse(security.verify_password("wrong password", password_hash))

    def test_session_token_uses_email_identity_when_github_identity_is_absent(self) -> None:
        user = User(
            id=42,
            email="creator@example.com",
            github_id=None,
            github_login=None,
            is_active=True,
        )

        payload = verify_signed_token(issue_session_token(user), expected_type="session")

        self.assertEqual(payload["sub"], "42")
        self.assertEqual(payload["login"], "creator@example.com")

    def test_user_read_accepts_email_only_and_platform_login_users(self) -> None:
        created_at = datetime.now(tz=timezone.utc)
        self.assertTrue(hasattr(User, "password_hash"))
        self.assertTrue(hasattr(User, "display_name"))
        self.assertTrue(hasattr(User, "platform_provider"))
        self.assertTrue(hasattr(User, "platform_openid"))

        email_user = SimpleNamespace(
            id=10,
            github_id=None,
            github_login=None,
            github_name=None,
            email="creator@example.com",
            password_hash="pbkdf2_sha256$1$00$00",
            display_name="内容创作者",
            avatar_url=None,
            platform_provider=None,
            platform_openid=None,
            is_active=True,
            last_login_at=None,
            created_at=created_at,
            updated_at=created_at,
        )
        miniapp_user = SimpleNamespace(
            id=11,
            github_id=None,
            github_login=None,
            github_name=None,
            email=None,
            password_hash=None,
            avatar_url=None,
            platform_provider="wechat",
            platform_openid="openid-123",
            display_name="小程序用户",
            is_active=True,
            last_login_at=None,
            created_at=created_at,
            updated_at=created_at,
        )

        try:
            email_read = UserRead.model_validate(email_user)
            miniapp_read = UserRead.model_validate(miniapp_user)
        except Exception as exc:  # pragma: no cover - assertion keeps RED readable.
            self.fail(f"UserRead should accept non-GitHub HotKey identities: {exc}")

        self.assertIsNone(email_read.github_id)
        self.assertEqual(email_read.email, "creator@example.com")
        self.assertEqual(email_read.display_name, "内容创作者")
        self.assertEqual(miniapp_read.platform_provider, "wechat")
        self.assertEqual(miniapp_read.platform_openid, "openid-123")


if __name__ == "__main__":
    unittest.main()
