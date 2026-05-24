from __future__ import annotations

import unittest
from datetime import date, datetime, timezone
from types import SimpleNamespace
from unittest.mock import patch
from xml.etree.ElementTree import fromstring as parse_xml

from fastapi.testclient import TestClient
from sqlalchemy import select
from sqlalchemy.dialects import postgresql

from apps.api.app.core.security import issue_signed_token, issue_session_token, parse_session_token, verify_signed_token
from apps.api.app.core.middleware import reset_request_metrics
from apps.api.app.core.settings import settings
from apps.api.app.main import create_app
from apps.api.app.db.session import SessionLocal
from apps.api.app.models.ai_analysis import AiAnalysis
from apps.api.app.models.hotspot import Hotspot
from apps.api.app.models.keyword import Keyword
from apps.api.app.models.user import User
from apps.api.app.models.report import Report
from apps.api.app.models.source import Source
from apps.api.app.services.ai_analysis import AnalysisResult, analyze_hotspot, expand_keyword_queries, is_analysis_active
from apps.api.app.services.ai.providers.openai import OpenAICompatibleProvider
from apps.api.app.services.ingestion import Candidate, SourceIngestionError, fetch_candidates
from apps.api.app.services.notification import notify_hotspot, notify_report
from apps.api.app.services.check_runner import _normalize_url
from apps.api.app.services.check_runner import run_hotspot_check
from apps.api.app.services.scheduler import _maybe_run_weekly_report
import apps.api.app.services.scheduler as scheduler_module
from apps.api.app.services.reports import previous_weekly_period_start, report_period
from apps.api.app.services.search import search_sources, _load_search_sources
from apps.api.app.services import rss as rss_service
from apps.api.app.services.providers import get_provider_class
from apps.api.app.schemas.ai_analysis import AiAnalysisRead
from apps.api.app.schemas.hotspot import HotspotRead
from apps.api.app.api.routes.hotspots import _apply_sort


class CollectingSession:
    def __init__(self) -> None:
        self.added: list[object] = []

    def add(self, item: object) -> None:
        self.added.append(item)


class ReadOnlySession:
    def add(self, item: object) -> None:
        raise AssertionError(f"search_sources must not persist {item!r}")


class FakeSessionForRun:
    def __init__(self) -> None:
        self.added: list[object] = []
        self.refreshed: list[object] = []

    def scalars(self, *_args: object, **_kwargs: object) -> list[object]:
        return []

    def add(self, item: object) -> None:
        self.added.append(item)

    def flush(self) -> None:
        return None

    def commit(self) -> None:
        return None

    def refresh(self, obj: object) -> None:
        self.refreshed.append(obj)


class FakeSessionForScalar:
    def __init__(self, report: Report | None = None) -> None:
        self._report = report

    def scalar(self, _statement: object) -> Report | None:
        return self._report


class SettingsPatchMixin:
    def patch_settings(self, **values: object) -> None:
        originals = {key: getattr(settings, key) for key in values}
        for key, value in values.items():
            setattr(settings, key, value)
        self.addCleanup(lambda: [setattr(settings, key, value) for key, value in originals.items()])

    def _auth_headers(self) -> dict[str, str]:
        with SessionLocal() as db:
            user = db.scalar(select(User).where(User.github_id == 987654321))
            if user is None:
                user = User(
                    github_id=987654321,
                    github_login="test-ci-user",
                    github_name="CI User",
                    email="ci@example.com",
                    avatar_url=None,
                    is_active=True,
                )
                db.add(user)
                db.commit()
                db.refresh(user)
            token = issue_session_token(user)
            return {"Authorization": f"Bearer {token}"}


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

    def test_fallback_analysis_returns_quick_understanding_and_topic_ideas(self) -> None:
        self.patch_settings(openai_api_key=None, openai_model=None)
        keyword = Keyword(id=1, keyword="AI agent", query_template=None, enabled=True, priority=0)
        hotspot = Hotspot(
            id=12,
            title="AI agent workflows become a creator trend",
            url="https://example.com/agent-workflows",
            source_id=1,
            keyword_id=1,
            snippet="Creators are explaining how agent workflows change daily production.",
            raw_payload={},
        )

        result = analyze_hotspot(hotspot, keyword)

        self.assertGreaterEqual(len(result.quick_understanding), 2)
        self.assertGreaterEqual(len(result.topic_ideas), 2)
        self.assertIn("title", result.topic_ideas[0])
        self.assertIn("angle", result.topic_ideas[0])
        self.assertIn("format", result.topic_ideas[0])
        self.assertIn("rationale", result.topic_ideas[0])

    def test_ai_analysis_read_exposes_creator_understanding_fields_from_raw_response(self) -> None:
        created_at = datetime.now(tz=timezone.utc)
        analysis = AiAnalysis(
            id=1,
            hotspot_id=12,
            is_real=True,
            relevance_score=82,
            relevance_reason="与创作者工具高度相关。",
            keyword_mentioned=True,
            importance="high",
            summary="AI agent 工作流成为创作者热点。",
            model_name="fallback",
            raw_response={
                "quick_understanding": ["一句话看懂", "为什么重要"],
                "topic_ideas": [
                    {
                        "title": "AI agent 工作流怎么用",
                        "angle": "实操教程",
                        "format": "图文",
                        "rationale": "创作者可直接复用为选题。",
                    }
                ],
            },
            created_at=created_at,
            updated_at=created_at,
        )

        payload = AiAnalysisRead.model_validate(analysis)

        self.assertEqual(payload.quick_understanding, ["一句话看懂", "为什么重要"])
        self.assertEqual(payload.topic_ideas[0].title, "AI agent 工作流怎么用")

    def test_hotspot_read_exposes_ranking_trend_and_cluster_fields(self) -> None:
        created_at = datetime.now(tz=timezone.utc)
        hotspot = Hotspot(
            id=13,
            title="AI agent trend",
            url="https://example.com/agent-trend",
            source_id=1,
            keyword_id=1,
            snippet="AI agent trend details.",
            status="active",
            raw_payload={"cluster_id": "cluster-ai-agent", "trend_score": 64},
            fetched_at=created_at,
            created_at=created_at,
            updated_at=created_at,
        )
        hotspot.ai_analysis = AiAnalysis(  # type: ignore[assignment]
            id=2,
            hotspot_id=13,
            is_real=True,
            relevance_score=90,
            relevance_reason="高相关。",
            keyword_mentioned=True,
            importance="high",
            summary="摘要",
            model_name="fallback",
            raw_response={},
            created_at=created_at,
            updated_at=created_at,
        )

        payload = HotspotRead.model_validate(hotspot)

        self.assertEqual(payload.cluster_id, "cluster-ai-agent")
        self.assertEqual(payload.trend_score, 64)
        self.assertGreater(payload.rank_score, payload.trend_score)

    def test_hotspot_sort_contract_supports_rank_and_trend_desc(self) -> None:
        rank_stmt = _apply_sort(select(Hotspot), "rank_score_desc")
        trend_stmt = _apply_sort(select(Hotspot), "trend_score_desc")

        rank_sql = str(rank_stmt.compile(dialect=postgresql.dialect(), compile_kwargs={"literal_binds": True}))
        trend_sql = str(trend_stmt.compile(dialect=postgresql.dialect(), compile_kwargs={"literal_binds": True}))

        self.assertIn("relevance_score", rank_sql)
        self.assertIn("trend_score", trend_sql)

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
        self.assertEqual(get_provider_class("github-trending").source_type, "github_trending")
        self.assertEqual(get_provider_class("hacker-news").source_type, "hacker_news")
        self.assertEqual(get_provider_class("x-twitter").source_type, "x_twitter")
        self.assertEqual(get_provider_class("bili").source_type, "bilibili")
        self.assertEqual(get_provider_class("weibo_sogou").source_type, "sogou")

    def test_github_trending_provider_normalizes_repository_search_results(self) -> None:
        from apps.api.app.services.providers.github_trending import _fetch_github_trending

        payload = {
            "items": [
                {
                    "id": 123,
                    "full_name": "openai/agents",
                    "html_url": "https://github.com/openai/agents",
                    "description": "Build agentic applications.",
                    "stargazers_count": 42000,
                    "forks_count": 1200,
                    "language": "TypeScript",
                    "pushed_at": "2026-05-24T08:00:00Z",
                    "owner": {"login": "openai"},
                    "topics": ["ai", "agents"],
                }
            ]
        }
        response = SimpleNamespace(
            raise_for_status=lambda: None,
            json=lambda: payload,
        )

        with patch("apps.api.app.services.providers.github_trending.httpx.get", return_value=response) as request:
            candidates = _fetch_github_trending({"limit": 5, "language": "TypeScript"}, 9, 3, "AI agent")

        request.assert_called_once()
        params = request.call_args.kwargs["params"]
        self.assertIn("AI agent", params["q"])
        self.assertIn("language:TypeScript", params["q"])
        self.assertEqual(candidates[0].title, "openai/agents")
        self.assertEqual(candidates[0].url, "https://github.com/openai/agents")
        self.assertEqual(candidates[0].author, "openai")
        self.assertEqual(candidates[0].source_id, 9)
        self.assertEqual(candidates[0].keyword_id, 3)
        self.assertEqual(candidates[0].raw_payload["source_type"], "github_trending")
        self.assertEqual(candidates[0].raw_payload["stars"], 42000)

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

    def test_openai_provider_normalizes_case_insensitive_keys(self) -> None:
        self.patch_settings(
            ai_provider="deepsEek",
            deepseek_api_key="deepseek-key",
            deepseek_base_url="https://api.deepseek.com/v1",
            gemini_api_key="gemini-key",
            gemini_base_url="https://generativelanguage.googleapis.com",
            openai_api_key="openai-key",
            openai_base_url="https://api.openai.com/v1",
        )
        provider = OpenAICompatibleProvider()

        self.assertEqual(provider._resolve_model("DEEPSEEK"), "deepseek-chat")
        self.assertEqual(provider._resolve_model("Gemini"), settings.gemini_model or "gemini-pro")
        self.assertEqual(provider._base_url("DEEPSEEK"), "https://api.deepseek.com/v1")
        self.assertEqual(provider._api_key_and_url("GEMINI")[0], "gemini-key")

    def test_check_runner_normalize_url_removes_tracking_params_and_preserves_non_http(self) -> None:
        normalized = _normalize_url("https://example.com/news/abc/?utm_source=qq&q=1")
        self.assertEqual(normalized, "https://example.com/news/abc?q=1")
        self.assertEqual(_normalize_url("mailto:test@example.com"), "mailto:test@example.com")
        self.assertEqual(_normalize_url("relative/path"), "relative/path")
        self.assertEqual(_normalize_url("https://example.com:443/News/?utm_source=1&Q=2"), "https://example.com/news?q=2")

    def test_rate_limit_middleware_blocks_excessive_requests(self) -> None:
        self.patch_settings(rate_limit_per_minute=2)
        app = create_app()
        with TestClient(app) as client:
            responses = [client.get("/api/health") for _ in range(3)]
        statuses = [response.status_code for response in responses]
        self.assertEqual(statuses[:2], [200, 200])
        self.assertEqual(statuses[2], 429)
        self.assertEqual(responses[2].json()["error"]["code"], "rate_limit")

    def test_rate_limit_middleware_uses_forwarded_for(self) -> None:
        self.patch_settings(rate_limit_per_minute=1)
        app = create_app()
        with TestClient(app) as client:
            first = client.get("/api/health", headers={"X-Forwarded-For": "203.0.113.17"})
            second = client.get("/api/health", headers={"X-Forwarded-For": "203.0.113.17"})
            third = client.get("/api/health", headers={"X-Forwarded-For": "198.51.100.9"})
        self.assertEqual(first.status_code, 200)
        self.assertEqual(second.status_code, 429)
        self.assertEqual(second.json()["error"]["code"], "rate_limit")
        self.assertEqual(third.status_code, 200)

    def test_ops_metrics_includes_request_and_status_counters(self) -> None:
        reset_request_metrics()
        app = create_app()
        with TestClient(app) as client:
            first = client.get("/api/health")
            metrics_resp = client.get("/api/ops/metrics", headers=self._auth_headers())

        self.assertEqual(first.status_code, 200)
        self.assertEqual(metrics_resp.status_code, 200)
        metrics = metrics_resp.json()["metrics"]
        self.assertEqual(metrics["total_requests"], 1)
        self.assertIn("200", metrics["status_buckets"])
        self.assertEqual(metrics["status_buckets"]["200"], 1)
        self.assertEqual(metrics["status_by_class"]["2xx"], 1)

    def test_ops_metrics_counts_rate_limit_blocks(self) -> None:
        reset_request_metrics()
        self.patch_settings(rate_limit_per_minute=1)
        app = create_app()
        with TestClient(app) as client:
            first = client.get("/api/health", headers={"X-Forwarded-For": "192.0.2.9"})
            second = client.get("/api/health", headers={"X-Forwarded-For": "192.0.2.9"})
            metrics_resp = client.get("/api/ops/metrics", headers=self._auth_headers())

        self.assertEqual(first.status_code, 200)
        self.assertEqual(second.status_code, 429)
        self.assertEqual(metrics_resp.status_code, 200)
        metrics = metrics_resp.json()["metrics"]
        self.assertEqual(metrics["status_buckets"]["429"], 1)
        self.assertEqual(metrics["rate_limit_exceeded_total"], 1)
        self.assertGreaterEqual(metrics["active_rate_limit_clients"], 1)

    def test_analytics_endpoints_return_aggregated_data(self) -> None:
        app = create_app()
        trend = [{"date": "2026-05-01", "total_count": 3, "active_count": 2, "filtered_count": 1}]
        sources = [
            {
                "source_id": 1,
                "source_name": "Hacker News",
                "hotspot_count": 3,
                "active_count": 2,
                "filtered_count": 1,
            }
        ]
        sentiment = {"high": 2, "medium": 1, "low": 0}
        with (
            patch("apps.api.app.api.routes.analytics.analytics_service.get_trend", return_value=trend),
            patch("apps.api.app.api.routes.analytics.analytics_service.get_top_sources", return_value=sources),
            patch("apps.api.app.api.routes.analytics.analytics_service.get_sentiment", return_value=sentiment),
            TestClient(app) as client,
        ):
            trend_response = client.get("/api/analytics/trend?days=7", headers=self._auth_headers())
            source_response = client.get("/api/analytics/sources?days=7&limit=5", headers=self._auth_headers())
            sentiment_response = client.get("/api/analytics/sentiment?days=7", headers=self._auth_headers())

        self.assertEqual(trend_response.status_code, 200)
        self.assertEqual(source_response.status_code, 200)
        self.assertEqual(sentiment_response.status_code, 200)
        self.assertEqual(trend_response.json()["points"][0]["active_count"], 2)
        self.assertEqual(source_response.json()["items"][0]["hotspot_count"], 3)
        self.assertEqual(sentiment_response.json()["total"], 3)

    def test_error_response_is_structured_and_hides_stacktrace(self) -> None:
        with TestClient(create_app()) as client:
            resp = client.get("/api/notifications?limit=0", headers=self._auth_headers())
        self.assertEqual(resp.status_code, 422)
        payload = resp.json()
        self.assertEqual(payload["error"]["code"], "validation_error")
        self.assertEqual(payload["error"]["message"], "请求参数校验失败。")

    def test_run_hotspot_check_records_failure_when_keywords_or_sources_missing(self) -> None:
        session = FakeSessionForRun()
        check_run = run_hotspot_check(session, trigger_type="manual")

        self.assertEqual(check_run.status, "completed_with_errors")
        self.assertEqual(check_run.failure_count, 2)
        self.assertIsNotNone(check_run.error_summary)
        self.assertIn("No enabled keywords.", check_run.error_summary or "")
        self.assertIn("No enabled sources.", check_run.error_summary or "")

    def test_weekly_report_only_runs_on_target_day_and_hour(self) -> None:
        scheduler_module._last_weekly_report_start = None
        collected = SimpleNamespace(calls=[])

        class Session:
            def commit(self) -> None:
                collected.calls.append("commit")

        class FixedDateTime:
            @classmethod
            def now(cls, tz: object = None) -> datetime:
                return datetime(2026, 4, 25, 10, 30, tzinfo=timezone.utc)

        self.patch_settings(weekly_report_enabled=True, weekly_report_weekday=6, weekly_report_hour=9)
        with (
            patch("apps.api.app.services.scheduler.datetime", FixedDateTime),
            patch("apps.api.app.services.scheduler.generate_and_send_report", lambda *args, **kwargs: collected.calls.append("send")),
        ):
            _maybe_run_weekly_report(Session())

        self.assertIn("send", collected.calls)
        self.assertIn("commit", collected.calls)

    def test_load_search_sources_normalizes_source_type_alias(self) -> None:
        class Session:
            def __init__(self) -> None:
                self.statement_text: str = ""

            def scalars(self, stmt: object) -> list[object]:
                self.statement_text = str(stmt.compile(dialect=postgresql.dialect(), compile_kwargs={"literal_binds": True}))
                return []

        session = Session()
        _load_search_sources(session, ["x-twitter"])
        self.assertIn("x_twitter", session.statement_text)

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

    def test_ai_summary_rss_link_uses_absolute_url(self) -> None:
        report = Report(
            id=88,
            report_type="daily",
            period_start=datetime(2026, 4, 25, tzinfo=timezone.utc),
            period_end=datetime(2026, 4, 26, tzinfo=timezone.utc),
            status="generated",
            subject="AI 热点日报",
            summary="本期摘要",
            content="# AI 热点日报",
            hotspot_count=0,
        )
        session = FakeSessionForScalar(report=report)
        xml_content = rss_service.generate_ai_summary_feed(session, base_url="https://news.example.com")
        root = parse_xml(xml_content)

        links = root.findall("channel/item/link")
        self.assertEqual(len(links), 1)
        self.assertEqual(links[0].text, "https://news.example.com/api/reports/88")

    def test_session_token_roundtrip(self) -> None:
        payload = issue_signed_token({"type": "unit-test", "sub": "1"}, ttl_seconds=60)
        decoded = verify_signed_token(payload)
        self.assertEqual(decoded["type"], "unit-test")

    def test_api_endpoints_require_auth(self) -> None:
        app = create_app()
        with TestClient(app) as client:
            response = client.get("/api/keywords")
        self.assertEqual(response.status_code, 401)

    def test_issue_and_validate_session_token(self) -> None:
        user = User(
            id=1,
            github_id=123456,
            github_login="tester",
            github_name="Tester",
            email="tester@example.com",
            avatar_url=None,
            is_active=True,
        )
        token = issue_session_token(user)
        subject = parse_session_token(token)
        self.assertEqual(subject, user.id)


if __name__ == "__main__":
    unittest.main()
