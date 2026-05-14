from __future__ import annotations

import unittest
from datetime import date, datetime, timezone
from unittest.mock import patch
from xml.etree.ElementTree import fromstring as parse_xml

from apps.api.app.core.settings import settings
from apps.api.app.main import create_app
from apps.api.app.models.ai_analysis import AiAnalysis
from apps.api.app.models.hotspot import Hotspot
from apps.api.app.models.keyword import Keyword
from apps.api.app.models.report import Report
from apps.api.app.models.source import Source
from apps.api.app.services.ai_analysis import AnalysisResult, analyze_hotspot, expand_keyword_queries, is_analysis_active
from apps.api.app.services.ingestion import Candidate, SourceIngestionError, fetch_candidates
from apps.api.app.services.notification import notify_hotspot, notify_report
from apps.api.app.services.check_runner import _normalize_url
from apps.api.app.services.reports import previous_weekly_period_start, report_period
from apps.api.app.services.search import search_sources
from apps.api.app.services.providers import get_provider_class


class CollectingSession:
    def __init__(self) -> None:
        self.added: list[object] = []

    def add(self, item: object) -> None:
        self.added.append(item)


class ReadOnlySession:
    def add(self, item: object) -> None:
        raise AssertionError(f"search_sources must not persist {item!r}")


class SettingsPatchMixin:
    def patch_settings(self, **values: object) -> None:
        originals = {key: getattr(settings, key) for key in values}
        for key, value in values.items():
            setattr(settings, key, value)
        self.addCleanup(lambda: [setattr(settings, key, value) for key, value in originals.items()])


class MvpServiceTests(SettingsPatchMixin, unittest.TestCase):
    def test_fallback_query_expansion_returns_two_to_five_unique_queries(self) -> None:
        self.patch_settings(openai_api_key=None, openai_model=None)
        keyword = Keyword(id=1, keyword="AI agent", query_template=None, enabled=True, priority=0)

        queries = expand_keyword_queries(keyword)

        self.assertGreaterEqual(len(queries), 2)
        self.assertLessEqual(len(queries), 5)
        self.assertEqual(len(queries), len(set(query.lower() for query in queries)))
        self.assertEqual(queries[0], "AI agent")

    def test_fallback_analysis_marks_keyword_mentions_above_threshold(self) -> None:
        self.patch_settings(openai_api_key=None, openai_model=None)
        keyword = Keyword(id=1, keyword="OpenAI", query_template=None, enabled=True, priority=0)
        hotspot = Hotspot(
            id=10,
            title="OpenAI launches new agent tooling",
            url="https://example.com/openai-agent",
            source_id=1,
            keyword_id=1,
            snippet="OpenAI released new tools for agent builders.",
            raw_payload={},
        )

        result = analyze_hotspot(hotspot, keyword)

        self.assertTrue(result.keyword_mentioned)
        self.assertGreaterEqual(result.relevance_score, settings.relevance_threshold)
        self.assertEqual(result.importance, "high")
        self.assertTrue(result.used_fallback)

    def test_fallback_analysis_marks_missing_keyword_below_threshold(self) -> None:
        self.patch_settings(openai_api_key=None, openai_model=None, relevance_threshold=50.0)
        keyword = Keyword(id=1, keyword="OpenAI", query_template=None, enabled=True, priority=0)
        hotspot = Hotspot(
            id=11,
            title="A database release",
            url="https://example.com/database",
            source_id=1,
            keyword_id=1,
            snippet="A database project shipped a maintenance release.",
            raw_payload={},
        )

        result = analyze_hotspot(hotspot, keyword)

        self.assertFalse(result.keyword_mentioned)
        self.assertLess(result.relevance_score, settings.relevance_threshold)
        self.assertEqual(result.importance, "low")

    def test_false_analysis_is_not_active_even_above_threshold(self) -> None:
        self.patch_settings(relevance_threshold=50.0)
        result = AnalysisResult(
            is_real=False,
            relevance_score=95,
            relevance_reason="来源不可信。",
            keyword_mentioned=True,
            importance="high",
            summary="疑似虚假信息。",
            model_name="test",
            raw_response={},
        )

        self.assertFalse(is_analysis_active(result))

    def test_optional_x_and_bing_sources_skip_without_credentials(self) -> None:
        self.patch_settings(x_api_bearer_token=None, bing_search_api_key=None)
        keyword = Keyword(id=1, keyword="AI", query_template=None, enabled=True, priority=0)
        x_source = Source(id=1, name="X/Twitter", source_type="x_twitter", enabled=True, config={})
        bing_source = Source(id=2, name="Bing", source_type="bing", enabled=True, config={})

        with self.assertRaisesRegex(SourceIngestionError, "X_API_BEARER_TOKEN"):
            fetch_candidates(x_source, keyword)

        with self.assertRaisesRegex(SourceIngestionError, "BING_SEARCH_API_KEY"):
            fetch_candidates(bing_source, keyword)

    def test_provider_registry_has_default_adapters(self) -> None:
        self.assertEqual(get_provider_class("rss").source_type, "rss")
        self.assertEqual(get_provider_class("hacker-news").source_type, "hacker_news")
        self.assertEqual(get_provider_class("x-twitter").source_type, "x_twitter")
        self.assertEqual(get_provider_class("bili").source_type, "bilibili")
        self.assertEqual(get_provider_class("weibo_sogou").source_type, "sogou")

    def test_fetch_candidates_uses_registered_provider_implementation(self) -> None:
        source = Source(id=10, name="Mock RSS", source_type="rss", enabled=True, config={"url": "https://example.com/rss"})
        keyword = Keyword(id=5, keyword="AI", query_template=None, enabled=True, priority=0)
        expected_candidate = Candidate(
            title="test title",
            url="https://example.com/news/1",
            source_id=10,
            keyword_id=5,
            author="alice",
            published_at=None,
            snippet="test",
            raw_payload={"source_type": "rss"},
        )

        with patch("apps.api.app.services.providers.rss._fetch_rss", return_value=[expected_candidate]):
            candidates = fetch_candidates(source, keyword)

        self.assertEqual(candidates, [expected_candidate])

    def test_smtp_missing_records_skipped_notification(self) -> None:
        self.patch_settings(smtp_host=None, smtp_from_email=None, smtp_to_email=None)
        session = CollectingSession()
        hotspot = Hotspot(id=20, title="OpenAI launch", url="https://example.com/openai", source_id=1, keyword_id=1, raw_payload={})
        analysis = AiAnalysis(
            hotspot_id=20,
            is_real=True,
            relevance_score=80,
            relevance_reason="关键词命中，相关性高。",
            keyword_mentioned=True,
            importance="high",
            summary="OpenAI 发布新产品。",
            model_name="local-fallback",
            raw_response={},
        )

        notification = notify_hotspot(session, hotspot, analysis)  # type: ignore[arg-type]

        self.assertEqual(notification.status, "skipped")
        self.assertEqual(notification.error_message, "SMTP is not configured.")
        self.assertEqual(session.added, [notification])

    def test_report_smtp_missing_records_skipped_notification(self) -> None:
        self.patch_settings(smtp_host=None, smtp_from_email=None, smtp_to_email=None)
        session = CollectingSession()
        report = Report(
            id=30,
            report_type="daily",
            period_start=datetime(2026, 4, 25, tzinfo=timezone.utc),
            period_end=datetime(2026, 4, 26, tzinfo=timezone.utc),
            subject="AI 热点日报",
            summary="本期摘要",
            content="# AI 热点日报",
            hotspot_count=0,
        )

        notification = notify_report(session, report)  # type: ignore[arg-type]

        self.assertEqual(notification.status, "skipped")
        self.assertEqual(notification.error_message, "SMTP is not configured.")
        self.assertEqual(notification.report_id, 30)
        self.assertEqual(session.added, [notification])

    def test_search_initializes_sources_and_does_not_persist_hotspots(self) -> None:
        self.patch_settings(openai_api_key=None, openai_model=None, relevance_threshold=50.0)
        source = Source(id=1, name="Hacker News", source_type="hacker_news", enabled=True, config={})
        candidate = Candidate(
            title="OpenAI ships agent search",
            url="https://example.com/agent-search",
            source_id=1,
            keyword_id=None,
            author="alice",
            published_at=datetime(2026, 4, 25, tzinfo=timezone.utc),
            snippet="OpenAI agent search tooling launched.",
            raw_payload={"id": "1"},
        )
        with (
            patch("apps.api.app.services.search.ensure_default_sources") as ensure_defaults,
            patch("apps.api.app.services.search._load_search_sources", return_value=[source]),
            patch("apps.api.app.services.search.expand_keyword_queries", return_value=["OpenAI agent"]),
            patch("apps.api.app.services.search.fetch_candidates", return_value=[candidate]),
        ):
            result = search_sources(ReadOnlySession(), "OpenAI agent")

        ensure_defaults.assert_called_once()
        self.assertEqual(len(result.items), 1)
        self.assertEqual(result.items[0].status, "active")
        self.assertEqual(result.errors, [])

    def test_report_period_supports_daily_and_weekly_defaults(self) -> None:
        daily_start, daily_end = report_period("daily", period_start=date(2026, 4, 25))
        weekly_start, weekly_end = report_period("weekly", period_start=date(2026, 4, 26))

        self.assertEqual(daily_start, datetime(2026, 4, 25, tzinfo=timezone.utc))
        self.assertEqual(daily_end, datetime(2026, 4, 26, tzinfo=timezone.utc))
        self.assertEqual(weekly_start, datetime(2026, 4, 20, tzinfo=timezone.utc))
        self.assertEqual(weekly_end, datetime(2026, 4, 27, tzinfo=timezone.utc))
        self.assertEqual(previous_weekly_period_start(datetime(2026, 4, 26, tzinfo=timezone.utc)), date(2026, 4, 13))

    def test_reports_routes_registered_and_daily_reports_removed(self) -> None:
        paths = {route.path for route in create_app().routes}

        self.assertIn("/api/reports", paths)
        self.assertIn("/api/reports/{report_id}", paths)
        self.assertIn("/api/reports/{report_id}/send", paths)
        self.assertIn("/api/reports/{report_id}/html", paths)
        self.assertNotIn("/api/daily-reports", paths)
        self.assertNotIn("/api/daily-reports/{report_id}/send", paths)

    def test_rss_routes_registered(self) -> None:
        paths = {route.path for route in create_app().routes}

        self.assertIn("/rss/trending", paths)
        self.assertIn("/rss/keyword/{keyword_name}", paths)
        self.assertIn("/rss/user/{user_id}", paths)

    def test_check_runner_normalize_url_removes_tracking_params_and_preserves_non_http(self) -> None:
        normalized = _normalize_url("https://example.com/news/abc/?utm_source=qq&q=1")
        self.assertEqual(normalized, "https://example.com/news/abc?q=1")
        self.assertEqual(_normalize_url("mailto:test@example.com"), "mailto:test@example.com")
        self.assertEqual(_normalize_url("relative/path"), "relative/path")

    def test_ai_analysis_falls_back_when_provider_call_fails(self) -> None:
        self.patch_settings(
            ai_provider="openai",
            openai_api_key=None,
            openai_model="gpt-4o-mini",
            deepseek_api_key=None,
            deepseek_model=None,
            gemini_api_key=None,
            gemini_model=None,
        )
        keyword = Keyword(id=1, keyword="AI agent", query_template=None, enabled=True, priority=0)
        hotspot = Hotspot(
            id=101,
            title="AI agent update",
            url="https://example.com/agent",
            source_id=1,
            keyword_id=1,
            snippet="AI agent update release notes.",
            raw_payload={},
        )

        result = analyze_hotspot(hotspot, keyword)

        self.assertTrue(result.used_fallback)
        self.assertEqual(result.provider, "fallback")
        self.assertIn("AI agent update release notes.", result.summary)

    def test_rss_render_feed_outputs_valid_xml(self) -> None:
        hotspot = Hotspot(
            id=1,
            title="AI test",
            url="https://example.com/1",
            source_id=1,
            keyword_id=1,
            snippet="test summary",
            raw_payload={},
            status="active",
        )
        analysis = AiAnalysis(
            hotspot_id=1,
            is_real=True,
            relevance_score=88,
            relevance_reason="",
            keyword_mentioned=True,
            importance="high",
            summary="AI 分析摘要",
            model_name="fallback",
            raw_response={},
        )
        hotspot.ai_analysis = analysis  # type: ignore[attr-defined]
        from apps.api.app.services import rss as rss_service

        xml_content = rss_service._render_feed("测试", "desc", [hotspot])
        root = parse_xml(xml_content)

        self.assertEqual(root.tag, "rss")
        self.assertEqual(root.findtext("channel/title"), "测试")


if __name__ == "__main__":
    unittest.main()
