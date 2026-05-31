import pathlib
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
MIGRATION = ROOT / "migrations" / "000003_channels_subscriptions.up.sql"
OPENAPI = ROOT / "docs" / "openapi.yaml"


class ChannelContractTest(unittest.TestCase):
    def test_channels_subscriptions_keywords_and_settings_schema_contract(self):
        sql = MIGRATION.read_text(encoding="utf-8")

        for required in [
            "CREATE TABLE IF NOT EXISTS channels",
            "user_channel_subscriptions",
            "user_keywords",
            "system_settings",
            "AI 模型",
            "AI 产品",
            "AI 开源",
            "AI 投融资",
            "default_daily_send_at",
        ]:
            self.assertIn(required, sql)

    def test_openapi_channel_paths_are_documented(self):
        spec = OPENAPI.read_text(encoding="utf-8")

        for path in [
            "/api/v1/channels:",
            "/api/v1/me/channels:",
            "/api/v1/me/channels/{channelID}:",
            "/api/v1/me/keywords:",
            "/api/v1/me/keywords/{keywordID}:",
            "/api/v1/me/preferences/daily-send-at:",
            "/api/v1/admin/channels:",
            "/api/v1/admin/channels/{channelID}:",
            "/api/v1/admin/settings/default-daily-send-at:",
        ]:
            self.assertIn(path, spec)

        for schema in [
            "Channel:",
            "Subscription:",
            "Keyword:",
            "DailySendAtRequest:",
            "channel_disabled",
            "channel_slug_already_exists",
        ]:
            self.assertIn(schema, spec)


if __name__ == "__main__":
    unittest.main()
