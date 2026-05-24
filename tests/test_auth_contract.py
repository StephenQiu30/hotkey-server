from __future__ import annotations

import unittest
from datetime import datetime, timezone
from pathlib import Path
from types import SimpleNamespace

from server.app.core import security
from server.app.core.security import issue_session_token, parse_session_token, verify_signed_token
from server.app.api.routes.auth import login_with_email, login_with_miniapp, register_with_email
from server.app.main import create_app
from server.app.models.user import User
from server.app.schemas.auth import EmailLoginRequest, EmailRegisterRequest, MiniappLoginRequest, UserRead


class FakeAuthSession:
    def __init__(self, scalar_results: list[User | None] | None = None) -> None:
        self.scalar_results = list(scalar_results or [])
        self.added: list[User] = []
        self.committed = False
        self.refreshed: list[User] = []

    def scalar(self, _statement: object) -> User | None:
        if self.scalar_results:
            return self.scalar_results.pop(0)
        return None

    def add(self, item: User) -> None:
        self.added.append(item)

    def commit(self) -> None:
        self.committed = True

    def refresh(self, user: User) -> None:
        if user.id is None:
            user.id = 100 + len(self.refreshed)
        now = datetime.now(tz=timezone.utc)
        if user.created_at is None:
            user.created_at = now
        if user.updated_at is None:
            user.updated_at = now
        self.refreshed.append(user)


class AuthContractTests(unittest.TestCase):
    def test_schema_bootstrap_migrates_existing_users_table_for_new_auth_fields(self) -> None:
        schema_sql = Path("sql/001_init_schema.sql").read_text(encoding="utf-8")

        self.assertIn("ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash", schema_sql)
        self.assertIn("ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name", schema_sql)
        self.assertIn("ALTER TABLE users ADD COLUMN IF NOT EXISTS platform_provider", schema_sql)
        self.assertIn("ALTER TABLE users ADD COLUMN IF NOT EXISTS platform_openid", schema_sql)
        self.assertIn("ALTER TABLE users ADD COLUMN IF NOT EXISTS role", schema_sql)

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
        self.assertTrue(hasattr(User, "role"))

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
        self.assertIsNone(email_read.role)
        self.assertIsNone(miniapp_read.role)

    def test_email_registration_creates_user_with_hashed_password_and_token(self) -> None:
        session = FakeAuthSession()

        response = register_with_email(
            EmailRegisterRequest(
                email=" Creator@Example.COM ",
                password="Correct Horse Battery Staple",
                display_name=" 创作者 ",
            ),
            session=session,  # type: ignore[arg-type]
        )

        self.assertTrue(session.committed)
        self.assertEqual(len(session.added), 1)
        created_user = session.added[0]
        self.assertEqual(created_user.email, "creator@example.com")
        self.assertEqual(created_user.display_name, "创作者")
        self.assertNotEqual(created_user.password_hash, "Correct Horse Battery Staple")
        self.assertTrue(security.verify_password("Correct Horse Battery Staple", created_user.password_hash))
        self.assertEqual(parse_session_token(response.access_token), created_user.id)

    def test_email_login_verifies_password_and_refreshes_last_login(self) -> None:
        existing_user = User(
            id=77,
            email="creator@example.com",
            password_hash=security.hash_password("Correct Horse Battery Staple"),
            display_name="创作者",
            is_active=True,
        )
        session = FakeAuthSession([existing_user])

        response = login_with_email(
            EmailLoginRequest(email="CREATOR@example.com", password="Correct Horse Battery Staple"),
            session=session,  # type: ignore[arg-type]
        )

        self.assertTrue(session.committed)
        self.assertIsNotNone(existing_user.last_login_at)
        self.assertEqual(parse_session_token(response.access_token), existing_user.id)

    def test_miniapp_login_creates_platform_identity_user(self) -> None:
        session = FakeAuthSession()

        response = login_with_miniapp(
            MiniappLoginRequest(
                provider=" WeChat ",
                openid="openid-123",
                display_name="小程序创作者",
                avatar_url="https://example.com/avatar.png",
            ),
            session=session,  # type: ignore[arg-type]
        )

        created_user = session.added[0]
        self.assertEqual(created_user.platform_provider, "wechat")
        self.assertEqual(created_user.platform_openid, "openid-123")
        self.assertEqual(created_user.display_name, "小程序创作者")
        self.assertEqual(parse_session_token(response.access_token), created_user.id)


if __name__ == "__main__":
    unittest.main()
