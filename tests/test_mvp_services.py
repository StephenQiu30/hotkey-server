from __future__ import annotations

import unittest
from inspect import signature
from datetime import date, datetime, timezone
from types import SimpleNamespace
from unittest.mock import patch
from xml.etree.ElementTree import fromstring as parse_xml

from fastapi.testclient import TestClient
from sqlalchemy import select
from sqlalchemy.dialects import postgresql

from server.app.core.security import issue_signed_token, issue_session_token, parse_session_token, verify_signed_token
from server.app.core.middleware import reset_request_metrics
from server.app.core.settings import settings
from server.app.main import create_app
from server.app.db.session import SessionLocal
from server.app.db.session import get_session
from server.app.models.ai_analysis import AiAnalysis
from server.app.models.notification import Notification
from server.app.models.hotspot import Hotspot
from server.app.models.keyword import Keyword
from server.app.models.user import User
from server.app.models.report import Report
from server.app.models.source import Source
from server.app.core.security import get_current_user
from server.app.api.routes.hotspots import get_hotspot_cluster, get_hotspot_cluster_history
from server.app.services.ai_analysis import AnalysisResult, analyze_hotspot, expand_keyword_queries, is_analysis_active
from server.app.services.ai.orchestrator import AIOrchestrator, LangGraphOrchestrator, build_orchestrator
from server.app.services.ai.providers.base import BaseLLMProvider, LLMResult
from server.app.services.ai.providers import build_provider
from server.app.services.ai.providers.openai import OpenAICompatibleProvider
from server.app.services.ingestion import Candidate, SourceIngestionError, fetch_candidates
from server.app.services.notification import notify_hotspot, notify_report
from server.app.services.check_runner import _build_analysis_raw_response, _decide_hotspot_status, _normalize_url, _source_route_payload
from server.app.services.check_runner import _estimate_cross_sources, _next_cluster_version, _should_enhance_analysis as check_runner_should_enhance
from server.app.services.check_runner import run_hotspot_check
from server.app.services.scheduler import _maybe_run_weekly_report
import server.app.services.scheduler as scheduler_module
from server.app.services.reports import previous_weekly_period_start, report_period
from server.app.services.search import _should_enhance_analysis as search_should_enhance
from server.app.services.search import _load_search_sources, search_sources
from server.app.services import rss as rss_service
from server.app.services.providers import get_provider_class
from server.app.services.providers.selector import mark_source_success, mark_source_failure, select_sources
from server.app.services.hotspot_scoring import compute_hotness_score
from server.app.services.source_trust import SourceEvidence, collect_source_evidence
from server.app.schemas.ai_analysis import AiAnalysisRead
from server.app.schemas.hotspot import HotspotRead
from server.app.api.routes.hotspots import _apply_sort, list_hotspots


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


class FakeSessionForCluster:
    def __init__(self, *, scalars_results: list[list[object]], scalar_results: list[int | Hotspot | None]) -> None:
        self.scalars_results = list(scalars_results)
        self.scalar_results = list(scalar_results)

    def scalars(self, *_args: object, **_kwargs: object) -> list[object]:
        if not self.scalars_results:
            return []
        items = self.scalars_results.pop(0)

        class _ScalarsResult(list[object]):
            def unique(self) -> "_ScalarsResult":
                return self

        return _ScalarsResult(items)

    def scalar(self, _statement: object) -> int | Hotspot | None:
        if not self.scalar_results:
            return None
        return self.scalar_results.pop(0)


class FakeSessionForPermissions:
    def __init__(self) -> None:
        self.added: list[object] = []
        self.committed = False

    def add(self, item: object) -> None:
        if hasattr(item, "id") and getattr(item, "id") is None:
            setattr(item, "id", 1000)
        now = datetime.now(tz=timezone.utc)
        for field_name in ("created_at", "updated_at", "fetched_at"):
            if hasattr(item, field_name) and getattr(item, field_name) is None:
                setattr(item, field_name, now)
        self.added.append(item)

    def add_all(self, items: list[object]) -> None:
        self.added.extend(items)

    def commit(self) -> None:
        self.committed = True

    def refresh(self, _item: object) -> None:
        return None

    def scalars(self, _statement: object) -> list[object]:
        return []

    def scalar(self, _statement: object) -> None:
        return None

    def rollback(self) -> None:
        return None


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

    def _app_with_user(self, role: str, session_override: object | None = None):
        app = create_app()
        user = User(
            id=10001,
            github_id=987654321,
            github_login="role-user",
            is_active=True,
            role=role,
        )
        app.dependency_overrides[get_current_user] = lambda: user
        app.dependency_overrides[get_session] = lambda: session_override or FakeSessionForPermissions()
        return app


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

    def test_hotspot_read_exposes_source_route_fields(self) -> None:
        created_at = datetime.now(tz=timezone.utc)
        hotspot = Hotspot(
            id=14,
            title="AI source fallback trend",
            url="https://example.com/source-fallback",
            source_id=1,
            keyword_id=1,
            snippet="Source fallback details.",
            status="active",
            raw_payload={
                "source_selected": "backup",
                "source_selected_type": "rss",
                "source_fallback": {
                    "from": "primary",
                    "to": "backup",
                    "reason": "timeout",
                },
            },
            fetched_at=created_at,
            created_at=created_at,
            updated_at=created_at,
        )

        payload = HotspotRead.model_validate(hotspot)

        self.assertEqual(payload.source_selected, "backup")
        self.assertEqual(payload.source_selected_type, "rss")
        self.assertEqual(payload.source_fallback["from"], "primary")
        self.assertEqual(payload.source_fallback["reason"], "timeout")

    def test_hotspot_read_falls_back_to_raw_payload_source_evidence(self) -> None:
        created_at = datetime.now(tz=timezone.utc)
        hotspot = Hotspot(
            id=15,
            title="AI evidence persistence",
            url="https://example.com/evidence",
            source_id=1,
            keyword_id=1,
            snippet="Evidence persisted in raw payload.",
            status="filtered",
            raw_payload={
                "source_risk_level": "low",
                "source_risk_tags": ["duplicate_query_param"],
                "source_evidence_bundle": {
                    "version": 1,
                    "cross_source_count": 3,
                    "status": "ok",
                },
                "source_evidence_version": 1,
            },
            fetched_at=created_at,
            created_at=created_at,
            updated_at=created_at,
        )

        payload = HotspotRead.model_validate(hotspot)

        self.assertEqual(payload.source_risk_level, "low")
        self.assertEqual(payload.source_risk_tags, ["duplicate_query_param"])
        self.assertEqual(payload.source_evidence_bundle["cross_source_count"], 3)
        self.assertEqual(payload.source_evidence_version, 1)

    def test_hotspot_read_marks_low_trust_source_for_display(self) -> None:
        created_at = datetime.now(tz=timezone.utc)
        hotspot = Hotspot(
            id=16,
            title="AI low trust display",
            url="https://example.com/low-trust",
            source_id=1,
            keyword_id=1,
            snippet="Low trust source should be visible but risk-marked.",
            status="filtered",
            raw_payload={
                "source_risk_level": "low",
                "source_risk_tags": ["shortlink"],
            },
            fetched_at=created_at,
            created_at=created_at,
            updated_at=created_at,
        )

        payload = HotspotRead.model_validate(hotspot)

        self.assertEqual(payload.source_risk_badge, "low_trust_source")
        self.assertIn("shortlink", payload.source_risk_tags)

    # PRD 24/25/26/27 traceability: these names intentionally mirror the Plan TDD checklist.
    def test_compute_hotness_clamps_to_0_100(self) -> None:
        source = Source(id=11, name="rss", source_type="rss", enabled=True, config={"source_strength": 120})
        hotspot = Hotspot(
            id=11,
            title="AI 热点",
            url="https://example.com/overflow",
            source_id=11,
            keyword_id=1,
            snippet="AI 流量",
            published_at=datetime(2026, 5, 24, tzinfo=timezone.utc),
            raw_payload={},
            source=source,
        )
        raw = SimpleNamespace(relevance_score=1000, keyword_mentioned=True)

        decision = compute_hotness_score(hotspot=hotspot, analysis=raw)

        self.assertGreaterEqual(decision.score, 0.0)
        self.assertLessEqual(decision.score, settings.hotness_max_score)

    def test_compute_hotness_uses_default_source_strength_without_source(self) -> None:
        self.patch_settings(hotness_source_strength_default=65.0)
        hotspot = Hotspot(
            id=111,
            title="AI 来源缺失边界",
            url="https://example.com/no-source",
            source_id=11,
            keyword_id=1,
            snippet="来源对象未预加载时仍需可复现打分。",
            published_at=datetime(2026, 5, 24, tzinfo=timezone.utc),
            raw_payload={},
        )
        raw = SimpleNamespace(relevance_score=80, keyword_mentioned=True)

        decision = compute_hotness_score(hotspot=hotspot, analysis=raw)

        self.assertEqual(decision.breakdown.source_strength, 65.0)
        self.assertGreater(decision.score, 0.0)
        self.assertIn("来源强度=65.00", decision.reason)

    def test_hotness_threshold_falls_back_and_records_filter_reason(self) -> None:
        self.patch_settings(hotness_active_threshold="bad-threshold")
        evidence = SourceEvidence(
            source_reachable=True,
            url_stability=True,
            domain_risk=90.0,
            publish_depth=100.0,
            cross_source_count=1,
            status="ok",
            risk_tags=[],
        )
        analysis = AnalysisResult(
            is_real=True,
            relevance_score=95,
            relevance_reason="high relevance",
            keyword_mentioned=True,
            importance="high",
            summary="summary",
            model_name="fallback",
            raw_response={},
        )
        hotness = SimpleNamespace(score=69.0, raw_payload=lambda: {"hotness_score": 69.0, "hotness_version": 1})

        status = _decide_hotspot_status(analysis, hotness, evidence)
        raw = _build_analysis_raw_response(analysis_result=analysis, evidence=evidence, hotness=hotness)

        self.assertEqual(status, "filtered")
        self.assertEqual(raw["hotness_active_threshold"], 70.0)
        self.assertEqual(raw["hotness_filter_reason"], "below_threshold")

    def test_hotness_high_relevance_marks_active(self) -> None:
        self.patch_settings(hotness_active_threshold=70.0, relevance_threshold=50.0)
        source = Source(id=12, name="hacker news", source_type="hacker_news", enabled=True, config={"source_strength": 80})
        hotspot = Hotspot(
            id=12,
            title="AI 热点上线",
            url="https://example.com/ai-01",
            source_id=12,
            keyword_id=1,
            snippet="AI 热点",
            published_at=datetime(2026, 5, 24, tzinfo=timezone.utc),
            raw_payload={},
            source=source,
        )
        evidence = SourceEvidence(
            source_reachable=True,
            url_stability=True,
            domain_risk=90.0,
            publish_depth=100.0,
            cross_source_count=1,
            status="ok",
            risk_tags=[],
        )
        analysis = AnalysisResult(
            is_real=True,
            relevance_score=96,
            relevance_reason="high relevance",
            keyword_mentioned=True,
            importance="high",
            summary="",
            model_name="fallback",
            raw_response={},
        )
        decision = compute_hotness_score(hotspot=hotspot, analysis=analysis)

        self.assertGreaterEqual(decision.score, settings.hotness_active_threshold)
        self.assertEqual(_decide_hotspot_status(analysis, decision, evidence), "active")

    def test_hotness_low_reliability_becomes_filtered(self) -> None:
        source = Source(id=13, name="rss", source_type="rss", enabled=True, config={"source_strength": 60})
        hotspot = Hotspot(
            id=13,
            title="AI 风险",
            url="https://example.com/ai-risk",
            source_id=13,
            keyword_id=1,
            snippet="AI 风险",
            raw_payload={},
            source=source,
        )
        analysis = AnalysisResult(
            is_real=True,
            relevance_score=90,
            relevance_reason="high relevance",
            keyword_mentioned=True,
            importance="high",
            summary="",
            model_name="fallback",
            raw_response={},
        )
        evidence = SourceEvidence(
            source_reachable=False,
            url_stability=False,
            domain_risk=20.0,
            publish_depth=0.0,
            cross_source_count=1,
            status="unreachable",
            risk_tags=["unreachable"],
        )
        decision = compute_hotness_score(hotspot=hotspot, analysis=analysis, trust_penalty=evidence.penalty())

        self.assertEqual(evidence.risk_level(), "low")
        self.assertEqual(_decide_hotspot_status(analysis, decision, evidence), "filtered")

    def test_search_results_sorted_by_hotness_score(self) -> None:
        stmt = _apply_sort(select(Hotspot), "hotness_score_desc")
        sql = str(stmt.compile(dialect=postgresql.dialect(), compile_kwargs={"literal_binds": True}))

        self.assertLess(sql.index("hotness_score"), sql.index("relevance_score"))
        self.assertIn("published_at", sql)

    def test_run_hotspot_check_stores_hotness_fields(self) -> None:
        session = FakeSessionForRun()
        keyword = Keyword(id=1, keyword="AI", query_template=None, enabled=True, priority=1)
        source = Source(id=1, name="Hacker News", source_type="hacker_news", enabled=True, config={})
        captured_candidate: dict[str, object] = {}
        candidate = Candidate(
            title="AI agent",
            url="https://example.com/ai-agent-hotness",
            source_id=1,
            keyword_id=1,
            author="alice",
            snippet="AI trend",
            raw_payload={},
            published_at=datetime(2026, 5, 20, tzinfo=timezone.utc),
        )
        raw_hotspot = Hotspot(
            id=999,
            title=candidate.title,
            url=candidate.url,
            source_id=candidate.source_id,
            keyword_id=candidate.keyword_id,
            raw_payload={},
        )
        raw_hotspot.source = source

        def _fake_scalars_select(_statement: object) -> list[object]:
            text = str(_statement)
            if "keywords" in text:
                return [keyword]
            if "sources" in text:
                return [source]
            return []

        def _fake_analyze(_hotspot: Hotspot, _keyword: Keyword, prefer_langgraph: bool = False) -> AnalysisResult:
            return AnalysisResult(
                is_real=True,
                relevance_score=90,
                relevance_reason="ok",
                keyword_mentioned=True,
                importance="high",
                summary="",
                model_name="fallback",
                raw_response={},
            )

        with (
            patch("server.app.services.check_runner._next_cluster_version", return_value=1),
            patch("server.app.services.check_runner.fetch_candidates", return_value=[candidate]),
            patch("server.app.services.check_runner._get_or_create_hotspot", return_value=raw_hotspot),
            patch("server.app.services.check_runner.analyze_hotspot", side_effect=_fake_analyze),
            patch("server.app.services.check_runner.notify_hotspot", return_value=Notification(
                hotspot_id=999,
                channel="email",
                status="sent",
            )),
        ):
            session.scalars = _fake_scalars_select  # type: ignore[assignment]
            run_hotspot_check(session)

        analysis = next(item for item in session.added if isinstance(item, AiAnalysis))
        self.assertIn("hotness_score", analysis.raw_response)
        self.assertEqual(analysis.raw_response["hotness_version"], 1)
        self.assertIn("source_risk_level", analysis.raw_response)

    def test_truth_score_allows_degraded_mode(self) -> None:
        self.patch_settings(openai_api_key=None, openai_model=None)
        evidence = collect_source_evidence(
            Hotspot(
                id=1,
                title="AI",
                url="mailto:test@example.com",
                source_id=1,
                keyword_id=1,
                raw_payload={},
            ),
            cross_source_count=2,
        )

        self.assertEqual(evidence.risk_level(), "low")
        self.assertEqual(evidence.bundle()["status"], "unreachable")

    def test_source_domain_risk_labeling(self) -> None:
        evidence = collect_source_evidence(
            Hotspot(
                id=2,
                title="AI",
                url="https://bit.ly/abc123",
                source_id=1,
                keyword_id=1,
                raw_payload={},
            ),
            cross_source_count=1,
        )

        self.assertEqual(evidence.domain_risk, 40.0)
        self.assertIn("shortlink", evidence.risk_tags)

    def test_source_evidence_marks_duplicate_query_parameter_pollution(self) -> None:
        evidence = collect_source_evidence(
            Hotspot(
                id=311,
                title="AI",
                url="https://example.com/hot?id=1&id=2",
                source_id=1,
                keyword_id=1,
                raw_payload={},
            ),
            cross_source_count=1,
        )

        self.assertFalse(evidence.url_stability)
        self.assertIn("duplicate_query_param", evidence.risk_tags)

    def test_cross_source_count_in_evidence(self) -> None:
        evidence1 = collect_source_evidence(
            Hotspot(
                id=1,
                title="AI",
                url="https://example.com",
                source_id=1,
                keyword_id=1,
                raw_payload={},
            ),
            cross_source_count=1,
        )
        evidence2 = collect_source_evidence(
            Hotspot(
                id=1,
                title="AI",
                url="https://example.com",
                source_id=1,
                keyword_id=1,
                raw_payload={},
            ),
            cross_source_count=4,
        )

        self.assertLess(evidence1.truth_score(), evidence2.truth_score())

    def test_low_trust_penalty_affects_hotness(self) -> None:
        self.patch_settings(low_trust_penalty=25.0)
        source = Source(id=1, name="rss", source_type="rss", enabled=True, config={"source_strength": 80})
        hotspot = Hotspot(
            id=1,
            title="AI",
            url="mailto:test@example.com",
            source_id=1,
            keyword_id=1,
            snippet="AI",
            raw_payload={},
            source=source,
        )
        evidence = collect_source_evidence(hotspot)
        raw = SimpleNamespace(relevance_score=90, keyword_mentioned=True)
        without_penalty = compute_hotness_score(hotspot=hotspot, analysis=raw)
        with_penalty = compute_hotness_score(hotspot=hotspot, analysis=raw, trust_penalty=evidence.penalty())

        self.assertEqual(evidence.risk_level(), "low")
        self.assertLess(with_penalty.score, without_penalty.score)

    def test_low_trust_penalty_bad_config_falls_back_to_zero(self) -> None:
        self.patch_settings(low_trust_penalty="bad-penalty")
        evidence = SourceEvidence(
            source_reachable=False,
            url_stability=False,
            domain_risk=20.0,
            publish_depth=0.0,
            cross_source_count=1,
            status="degraded",
            risk_tags=["shortlink"],
        )

        self.assertEqual(evidence.risk_level(), "low")
        self.assertEqual(evidence.penalty(), 0.0)

    def test_low_trust_event_filtered_for_notification(self) -> None:
        self.test_run_hotspot_check_marks_low_trust_event_as_filtered_and_skip_notify()

    def test_evidence_bundle_visible_in_search_and_list(self) -> None:
        self.patch_settings(openai_api_key=None, openai_model=None, relevance_threshold=50.0)
        source = Source(id=1, name="Hacker News", source_type="hacker_news", enabled=True, config={})
        class _SearchSession:
            def scalar(self, _statement: object) -> int:
                return 0
        candidate = Candidate(
            title="OpenAI ships",
            url="https://example.com/agent",
            source_id=1,
            keyword_id=None,
            author="alice",
            published_at=datetime(2026, 4, 25, tzinfo=timezone.utc),
            snippet="agent",
            raw_payload={"id": "1"},
        )
        with (
            patch("server.app.services.search.ensure_default_sources"),
            patch("server.app.services.search._load_search_sources", return_value=[source]),
            patch("server.app.services.search.expand_keyword_queries", return_value=["OpenAI agent"]),
            patch("server.app.services.search.fetch_candidates", return_value=[candidate]),
            patch("server.app.services.search.analyze_hotspot", return_value=AnalysisResult(
                is_real=True,
                relevance_score=95,
                relevance_reason="ok",
                keyword_mentioned=True,
                importance="high",
                summary="",
                model_name="fallback",
                raw_response={},
            )),
        ):
            result = search_sources(_SearchSession(), "OpenAI agent")

        self.assertEqual(len(result.items), 1)
        self.assertIn("source_evidence_bundle", result.items[0].raw_payload)

    def test_orchestrator_uses_langchain_by_default(self) -> None:
        self.patch_settings(ai_use_langgraph=False)
        orchestrator = build_orchestrator(build_provider("fallback"), use_langgraph=True)
        self.assertIsInstance(orchestrator, AIOrchestrator)

    def test_langgraph_disabled_by_default(self) -> None:
        self.patch_settings(ai_provider="fallback", ai_use_langgraph=False)
        result = analyze_hotspot(
            Hotspot(
                id=88,
                title="AI agent",
                url="https://example.com/a",
                source_id=1,
                keyword_id=1,
                snippet="AI agent",
                raw_payload={},
            ),
            keyword=Keyword(id=1, keyword="AI", query_template=None, enabled=True, priority=0),
            prefer_langgraph=True,
        )

        self.assertEqual(result.ai_orchestrator_decision, "langchain")

    def test_langgraph_trigger_routes_to_graph(self) -> None:
        self.patch_settings(ai_use_langgraph=True)
        evidence = SourceEvidence(
            source_reachable=True,
            url_stability=True,
            domain_risk=60,
            publish_depth=100,
            cross_source_count=2,
            status="ok",
            risk_tags=[],
        )

        self.assertTrue(check_runner_should_enhance(evidence, hotness_score=95.0, langgraph_enabled=settings.ai_use_langgraph))

    def test_langgraph_timeout_falls_back_to_chain(self) -> None:
        class SlowProvider(BaseLLMProvider):
            provider_name = "slow"

            def expand_queries(self, keyword: Keyword, base_query: str) -> list[str]:
                return [base_query]

            def analyze(self, hotspot: Hotspot, keyword: Keyword | None) -> LLMResult:
                return LLMResult(
                    is_real=True,
                    relevance_score=88,
                    relevance_reason="ok",
                    keyword_mentioned=True,
                    importance="high",
                    summary="",
                    model_name="slow",
                    raw_response={"provider": "slow"},
                    used_fallback=False,
                    prompt_name="analysis",
                    provider="slow",
                )

        provider = SlowProvider()
        orchestrator = LangGraphOrchestrator(provider)
        self.patch_settings(ai_langgraph_timeout_seconds=1)

        with patch("server.app.services.ai.orchestrator.time.perf_counter", side_effect=[0.0, 2.5, 3.0, 3.01]):
            result, decision = orchestrator.analyze(
                Hotspot(
                    id=1,
                    title="AI",
                    url="https://example.com",
                    source_id=1,
                    keyword_id=1,
                    snippet="",
                    raw_payload={},
                ),
                Keyword(id=1, keyword="AI", query_template=None, enabled=True, priority=0),
            )

        self.assertEqual(result.raw_response.get("provider"), "slow")
        self.assertEqual(decision.decision.get("path"), "langchain")
        self.assertTrue(decision.decision.get("langgraph_fallback"))

    def test_open_endpoint_works_with_langchain(self) -> None:
        self.patch_settings(ai_provider="fallback", ai_use_langgraph=False)
        hotspot = Hotspot(
            id=1,
            title="AI",
            url="https://example.com",
            source_id=1,
            keyword_id=1,
            snippet="",
            raw_payload={},
        )

        result = analyze_hotspot(hotspot, None, prefer_langgraph=False)

        self.assertEqual(result.ai_orchestrator_decision, "langchain")

    def test_source_route_skips_failing_source(self) -> None:
        class Session:
            def __init__(self) -> None:
                self.added: list[object] = []
                self.commits = 0
            def add(self, item: object) -> None:
                self.added.append(item)
            def add_all(self, items: list[object]) -> None:
                self.added.extend(items)
            def commit(self) -> None:
                self.commits += 1
            def flush(self) -> None:
                return None
            def refresh(self, _item: object) -> None:
                return None
            def scalars(self, statement: object) -> list[object]:
                text = str(statement)
                if "keywords" in text:
                    return [Keyword(id=1, keyword="AI", query_template=None, enabled=True, priority=1)]
                if "sources" in text:
                    return [
                        Source(id=1, name="bad", source_type="hacker_news", enabled=True, config={}),
                        Source(id=2, name="good", source_type="rss", enabled=True, config={"url": "https://example.com"}),
                    ]
                return []
            def scalar(self, statement: object) -> object | None:
                if "keywords" in str(statement):
                    return None
                if "sources" in str(statement):
                    return None
                return None

        session = Session()
        keyword = Keyword(id=1, keyword="AI", query_template=None, enabled=True, priority=1)
        candidate = Candidate(
            title="OK",
            url="https://example.com/ok",
            source_id=2,
            keyword_id=1,
            author="a",
            snippet="",
            raw_payload={},
            published_at=None,
        )

        def _fetch_side_effect(source: Source, keyword: Keyword, query: str | None = None, record_health: bool = False, timeout_seconds: float | None = None):
            if source.id == 1:
                raise SourceIngestionError("fetch failed")
            return [candidate]

        with (
            patch("server.app.services.check_runner._next_cluster_version", return_value=1),
            patch("server.app.services.check_runner.expand_keyword_queries", return_value=["AI"]),
            patch("server.app.services.check_runner.fetch_candidates", side_effect=_fetch_side_effect),
            patch("server.app.services.check_runner._get_or_create_hotspot", return_value=Hotspot(
                id=1,
                title=candidate.title,
                url=candidate.url,
                source_id=2,
                keyword_id=1,
                snippet="",
                raw_payload={},
            )),
            patch("server.app.services.check_runner.analyze_hotspot", return_value=AnalysisResult(
                is_real=True,
                relevance_score=90,
                relevance_reason="ok",
                keyword_mentioned=True,
                importance="high",
                summary="",
                model_name="fallback",
                raw_response={},
            )),
        ):
            result = run_hotspot_check(session)

        self.assertEqual(result.success_count, 1)
        self.assertEqual(result.failure_count, 1)

    def test_source_retry_threshold_triggers_fallback(self) -> None:
        source = Source(id=1, name="rss", source_type="rss", enabled=True, config={})

        for _ in range(settings.source_failure_threshold):
            mark_source_failure(source, reason="timeout")

        self.assertEqual(source.config["health"]["status"], "degraded")

    def test_source_fallback_preserves_primary_flow(self) -> None:
        self.test_source_route_skips_failing_source()

    def test_source_route_payload_records_selected_and_fallback(self) -> None:
        primary = Source(id=1, name="primary", source_type="rss", enabled=True, config={})
        backup = Source(id=2, name="backup", source_type="rss", enabled=True, config={})

        payload = _source_route_payload(backup, fallback_from=primary, fallback_reason="timeout")

        self.assertEqual(payload["source_selected"], "backup")
        self.assertEqual(payload["source_selected_type"], "rss")
        self.assertEqual(payload["source_fallback"]["from"], "primary")
        self.assertEqual(payload["source_fallback"]["to"], "backup")
        self.assertEqual(payload["source_fallback"]["reason"], "timeout")

    def test_check_runner_still_completes_when_one_source_fail(self) -> None:
        self.test_source_route_skips_failing_source()

    def test_source_rotation_keeps_cluster_and_dedup(self) -> None:
        session = FakeSessionForRun()
        existing = [1, 2]

        class DistinctScalars(list[int]):
            def all(self) -> list[int]:
                return list(self)

        session.scalars = lambda _statement: DistinctScalars(existing)  # type: ignore[assignment]

        self.assertEqual(_estimate_cross_sources(session, "https://example.com/a", 3, {}), 3)
    def test_next_cluster_version_increments_from_existing_records(self) -> None:
        # We keep this deterministic by testing helper via a tiny session shim.
        class ScalarOnlySession:
            def __init__(self, value: int) -> None:
                self._value = value

            def scalar(self, _statement: object) -> int:
                return self._value

        self.assertEqual(_next_cluster_version(ScalarOnlySession(3), "cluster-ai"), 4)

    def test_run_hotspot_check_stores_cluster_metadata(self) -> None:
        session = FakeSessionForRun()
        keyword = Keyword(id=1, keyword="AI", query_template=None, enabled=True, priority=1)
        source = Source(id=1, name="Hacker News", source_type="hacker_news", enabled=True, config={})
        captured_candidate: dict[str, object] = {}
        raw_hotspot = Hotspot(
            id=55,
            title="AI agent",
            url="https://example.com/ai-agent",
            source_id=1,
            keyword_id=1,
            raw_payload={},
        )

        def _fake_scalars_select(_statement: object) -> list[object]:
            text = str(_statement)
            if "keywords" in text:
                return [keyword]
            if "sources" in text:
                return [source]
            return []

        session.scalars = _fake_scalars_select  # type: ignore[method-assign]

        candidate = Candidate(
            title="AI agent",
            url="https://example.com/ai-agent",
            source_id=1,
            keyword_id=1,
            author="alice",
            snippet="AI trend",
            raw_payload={},
            published_at=None,
        )

        def _capture_create(_session: object, candidate: object) -> Hotspot:
            captured_candidate["candidate"] = candidate
            return raw_hotspot

        def _fake_analyze(_hotspot: Hotspot, _keyword: Keyword, prefer_langgraph: bool = False) -> AnalysisResult:
            return AnalysisResult(
                is_real=True,
                relevance_score=80,
                relevance_reason="ok",
                keyword_mentioned=True,
                importance="high",
                summary="",
                model_name="fallback",
                raw_response={},
            )

        with (
            patch("server.app.services.check_runner._next_cluster_version", return_value=2),
            patch("server.app.services.check_runner.fetch_candidates", return_value=[candidate]),
            patch("server.app.services.check_runner._get_or_create_hotspot", side_effect=_capture_create),
            patch("server.app.services.check_runner.analyze_hotspot", side_effect=_fake_analyze),
            patch("server.app.services.check_runner.notify_hotspot", return_value=Notification(
                hotspot_id=55,
                channel="email",
                status="sent",
            )),
        ):
            session.scalars = _fake_scalars_select  # type: ignore[assignment]
            check_run = run_hotspot_check(session)

        candidate_payload = captured_candidate["candidate"].raw_payload  # type: ignore[union-attr]
        self.assertIsInstance(candidate_payload.get("cluster_id"), str)
        self.assertEqual(len(str(candidate_payload.get("cluster_id"))), 36)
        self.assertEqual(candidate_payload.get("cluster_version"), 2)
        self.assertIn("clustered_at", candidate_payload)
        self.assertEqual(check_run.success_count, 1)

    def test_run_hotspot_check_marks_low_trust_event_as_filtered_and_skip_notify(self) -> None:
        self.patch_settings(openai_api_key=None, openai_model=None, hotness_active_threshold=40.0)
        session = FakeSessionForRun()
        keyword = Keyword(id=1, keyword="AI", query_template=None, enabled=True, priority=1)
        source = Source(id=1, name="Hacker News", source_type="hacker_news", enabled=True, config={})
        raw_hotspot = Hotspot(
            id=60,
            title="AI agent",
            url="https://example.com/ai-agent-low-trust",
            source_id=1,
            keyword_id=1,
            raw_payload={},
        )
        raw_hotspot.source = source

        def _fake_scalars_select(_statement: object) -> list[object]:
            text = str(_statement)
            if "keywords" in text:
                return [keyword]
            if "sources" in text:
                return [source]
            return []

        session.scalars = _fake_scalars_select  # type: ignore[method-assign]

        candidate = Candidate(
            title="AI agent",
            url="https://example.com/ai-agent-low-trust",
            source_id=1,
            keyword_id=1,
            author="alice",
            snippet="AI trend",
            raw_payload={},
            published_at=None,
        )

        def _fake_analyze(_hotspot: Hotspot, _keyword: Keyword, prefer_langgraph: bool = False) -> AnalysisResult:
            return AnalysisResult(
                is_real=True,
                relevance_score=95,
                relevance_reason="ok",
                keyword_mentioned=True,
                importance="high",
                summary="",
                model_name="fallback",
                raw_response={},
            )

        low_risk_evidence = SourceEvidence(
            source_reachable=False,
            url_stability=False,
            domain_risk=20.0,
            publish_depth=0.0,
            cross_source_count=1,
            status="unreachable",
            risk_tags=["unreachable"],
        )

        with (
            patch("server.app.services.check_runner._next_cluster_version", return_value=2),
            patch("server.app.services.check_runner.fetch_candidates", return_value=[candidate]),
            patch("server.app.services.check_runner._get_or_create_hotspot", return_value=raw_hotspot),
            patch("server.app.services.check_runner.analyze_hotspot", side_effect=_fake_analyze),
            patch("server.app.services.check_runner.collect_source_evidence", return_value=low_risk_evidence),
            patch("server.app.services.check_runner.notify_hotspot") as notify_hotspot_mock,
        ):
            session.scalars = _fake_scalars_select  # type: ignore[assignment]
            check_run = run_hotspot_check(session)

        self.assertEqual(raw_hotspot.status, "filtered")
        self.assertEqual(check_run.success_count, 1)
        notify_hotspot_mock.assert_not_called()

    def test_get_hotspot_cluster_route_reads_clustered_items(self) -> None:
        now = datetime.now(tz=timezone.utc)
        hotspot_a = Hotspot(
            id=1,
            title="A",
            url="https://example.com/a",
            source_id=1,
            keyword_id=1,
            raw_payload={"cluster_id": "cluster-1", "cluster_version": 1},
            status="active",
            fetched_at=now,
            created_at=now,
            updated_at=now,
        )
        hotspot_b = Hotspot(
            id=2,
            title="B",
            url="https://example.com/b",
            source_id=1,
            keyword_id=1,
            raw_payload={"cluster_id": "cluster-1", "cluster_version": 2},
            status="active",
            fetched_at=now,
            created_at=now,
            updated_at=now,
        )
        cluster_session = FakeSessionForCluster(scalars_results=[[hotspot_a, hotspot_b]], scalar_results=[2])

        response = get_hotspot_cluster("cluster-1", limit=50, offset=0, session=cluster_session)

        self.assertEqual(response.cluster_id, "cluster-1")
        self.assertEqual(response.cluster_size, 2)
        self.assertEqual(response.items[0].cluster_version, 1)
        self.assertEqual(response.items[1].cluster_version, 2)

    def test_get_hotspot_cluster_history_uses_hotspot_cluster(self) -> None:
        now = datetime.now(tz=timezone.utc)
        hotspot = Hotspot(
            id=3,
            title="C",
            url="https://example.com/c",
            source_id=1,
            keyword_id=1,
            raw_payload={"cluster_id": "cluster-1", "cluster_version": 5},
            status="active",
            fetched_at=now,
            created_at=now,
            updated_at=now,
        )
        items = [hotspot]
        history_session = FakeSessionForCluster(
            scalars_results=[items],
            scalar_results=[hotspot, 1],
        )

        response = get_hotspot_cluster_history(3, session=history_session)

        self.assertEqual(response.cluster_id, "cluster-1")
        self.assertEqual(response.cluster_size, 1)
        self.assertEqual(response.items[0].id, 3)

    def test_viewer_forbidden_to_create_admin_only_resources(self) -> None:
        fake_session = FakeSessionForPermissions()
        app = self._app_with_user("viewer", fake_session)
        payload = {
            "keyword": "AI",
            "query_template": None,
            "enabled": True,
            "priority": 0,
        }
        with TestClient(app) as client:
            response = client.post("/api/keywords", json=payload)

        self.assertEqual(response.status_code, 403)

    def test_admin_can_create_admin_only_resource(self) -> None:
        fake_session = FakeSessionForPermissions()
        app = self._app_with_user("admin", fake_session)
        payload = {
            "keyword": "AI",
            "query_template": None,
            "enabled": True,
            "priority": 0,
        }
        with TestClient(app) as client:
            response = client.post("/api/keywords", json=payload)

        self.assertEqual(response.status_code, 201)

    def test_hotspot_sort_contract_supports_rank_and_trend_desc(self) -> None:
        rank_stmt = _apply_sort(select(Hotspot), "rank_score_desc")
        trend_stmt = _apply_sort(select(Hotspot), "trend_score_desc")
        hotness_stmt = _apply_sort(select(Hotspot), "hotness_score_desc")

        rank_sql = str(rank_stmt.compile(dialect=postgresql.dialect(), compile_kwargs={"literal_binds": True}))
        trend_sql = str(trend_stmt.compile(dialect=postgresql.dialect(), compile_kwargs={"literal_binds": True}))
        hotness_sql = str(hotness_stmt.compile(dialect=postgresql.dialect(), compile_kwargs={"literal_binds": True}))

        self.assertIn("relevance_score", rank_sql)
        self.assertIn("trend_score", trend_sql)
        self.assertIn("hotness_score", hotness_sql)

    def test_list_hotspots_default_sort_is_hotness_score_desc(self) -> None:
        params = signature(list_hotspots).parameters
        self.assertEqual(params["sort"].default, "hotness_score_desc")

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
        from server.app.services.providers.github_trending import _fetch_github_trending

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

        with patch("server.app.services.providers.github_trending.httpx.get", return_value=response) as request:
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

        with patch("server.app.services.providers.rss._fetch_rss", return_value=[expected_candidate]):
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
            published_at=datetime.now(tz=timezone.utc),
            snippet="OpenAI agent search tooling launched.",
            raw_payload={"id": "1"},
        )

        def _fake_analyze(_hotspot: Hotspot, _keyword: Keyword, prefer_langgraph: bool = False) -> AnalysisResult:
            return AnalysisResult(
                is_real=True,
                relevance_score=95,
                relevance_reason="ok",
                keyword_mentioned=True,
                importance="high",
                summary="",
                model_name="fallback",
                raw_response={},
            )

        with (
            patch("server.app.services.search.ensure_default_sources") as ensure_defaults,
            patch("server.app.services.search._load_search_sources", return_value=[source]),
            patch("server.app.services.search.expand_keyword_queries", return_value=["OpenAI agent"]),
            patch("server.app.services.search.fetch_candidates", return_value=[candidate]),
            patch("server.app.services.search.analyze_hotspot", side_effect=_fake_analyze),
        ):
            result = search_sources(ReadOnlySession(), "OpenAI agent")

        ensure_defaults.assert_called_once()
        self.assertEqual(len(result.items), 1)
        self.assertEqual(result.items[0].status, "active")
        self.assertEqual(result.errors, [])

    def test_search_filters_low_trust_result(self) -> None:
        self.patch_settings(openai_api_key=None, openai_model=None, relevance_threshold=50.0)
        source = Source(id=1, name="Hacker News", source_type="hacker_news", enabled=True, config={})
        class _SearchSession:
            def scalar(self, _statement: object) -> int:
                return 0

        candidate = Candidate(
            title="OpenAI ships agent search",
            url="https://example.com/agent-search-low-trust",
            source_id=1,
            keyword_id=None,
            author="alice",
            published_at=datetime(2026, 4, 25, tzinfo=timezone.utc),
            snippet="OpenAI agent search tooling launched.",
            raw_payload={"id": "1"},
        )
        low_risk_evidence = SourceEvidence(
            source_reachable=False,
            url_stability=False,
            domain_risk=20.0,
            publish_depth=0.0,
            cross_source_count=1,
            status="unreachable",
            risk_tags=["unreachable"],
        )
        with (
            patch("server.app.services.search.ensure_default_sources"),
            patch("server.app.services.search._load_search_sources", return_value=[source]),
            patch("server.app.services.search.expand_keyword_queries", return_value=["OpenAI agent"]),
            patch("server.app.services.search.fetch_candidates", return_value=[candidate]),
            patch("server.app.services.search.collect_source_evidence", return_value=low_risk_evidence),
        ):
            result = search_sources(_SearchSession(), "OpenAI agent")

        self.assertEqual(len(result.items), 1)
        self.assertEqual(result.items[0].status, "filtered")

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
            patch("server.app.api.routes.analytics.analytics_service.get_trend", return_value=trend),
            patch("server.app.api.routes.analytics.analytics_service.get_top_sources", return_value=sources),
            patch("server.app.api.routes.analytics.analytics_service.get_sentiment", return_value=sentiment),
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

    def test_search_should_enhance_analysis_when_hot_and_risky_or_conflicted(self) -> None:
        self.patch_settings(ai_use_langgraph=True, ai_enhance_hotness_threshold=70.0, ai_enhance_risk_threshold=40.0)
        evidence = SourceEvidence(
            source_reachable=True,
            url_stability=True,
            domain_risk=50,
            publish_depth=80,
            cross_source_count=2,
            status="ok",
            risk_tags=[],
        )

        self.assertTrue(search_should_enhance(
            evidence,
            hotness_score=85,
            langgraph_enabled=True,
        ))
        self.assertFalse(search_should_enhance(
            evidence,
            hotness_score=60,
            langgraph_enabled=True,
        ))
        self.assertFalse(search_should_enhance(
            evidence,
            hotness_score=85,
            langgraph_enabled=False,
        ))

    def test_check_runner_should_enhance_analysis_requires_hotness_and_risk_or_conflict(self) -> None:
        self.patch_settings(ai_use_langgraph=True, ai_enhance_hotness_threshold=70.0, ai_enhance_risk_threshold=40.0)
        low_truth = SourceEvidence(
            source_reachable=True,
            url_stability=False,
            domain_risk=0,
            publish_depth=0,
            cross_source_count=1,
            status="ok",
            risk_tags=[],
        )
        conflict = SourceEvidence(
            source_reachable=True,
            url_stability=True,
            domain_risk=50,
            publish_depth=80,
            cross_source_count=3,
            status="ok",
            risk_tags=[],
        )

        self.assertTrue(check_runner_should_enhance(low_truth, hotness_score=75, langgraph_enabled=True))
        self.assertTrue(check_runner_should_enhance(conflict, hotness_score=75, langgraph_enabled=True))
        self.assertFalse(check_runner_should_enhance(low_truth, hotness_score=65, langgraph_enabled=True))

    def test_build_analysis_raw_response_merges_enrichment_fields(self) -> None:
        source = Source(id=2, name="hacker news", source_type="hacker_news", enabled=True, config={})
        hotspot = Hotspot(
            id=66,
            title="AI agent",
            url="https://example.com/ai",
            source_id=2,
            keyword_id=1,
            snippet="AI agent",
            raw_payload={},
            source=source,
        )
        analysis = AnalysisResult(
            is_real=True,
            relevance_score=92,
            relevance_reason="ok",
            keyword_mentioned=True,
            importance="high",
            summary="",
            model_name="fallback",
            raw_response={"provider": "fallback"},
            ai_orchestrator_decision="langgraph",
            enhance_path="enhanced",
            fallback_reason="fallback reason",
            trace_id="trace-001",
            provider_trace=[{"event": "final"}],
            quick_understanding=["q1", "q2"],
            topic_ideas=[{"title": "t", "angle": "a", "format": "f", "rationale": "r"}],
        )
        evidence = collect_source_evidence(hotspot)
        hotness = compute_hotness_score(
            hotspot=hotspot,
            analysis=analysis,
            source_strength=80,
        )

        raw = _build_analysis_raw_response(analysis_result=analysis, evidence=evidence, hotness=hotness)

        self.assertEqual(raw["ai_orchestrator_decision"], "langgraph")
        self.assertEqual(raw["enhance_path"], "enhanced")
        self.assertEqual(raw["trace_id"], "trace-001")
        self.assertEqual(raw["fallback_reason"], "fallback reason")
        self.assertEqual(raw["provider"], "fallback")
        self.assertEqual(raw["hotness_score"], hotness.score)
        self.assertEqual(raw["source_risk_level"], evidence.risk_level())
        self.assertIn("provider_trace", raw)
        self.assertEqual(raw["provider_trace"], [{"event": "final"}])
        self.assertEqual(raw["prompt_name"], None)

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
            patch("server.app.services.scheduler.datetime", FixedDateTime),
            patch("server.app.services.scheduler.generate_and_send_report", lambda *args, **kwargs: collected.calls.append("send")),
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

    def test_ai_orchestrator_defaults_to_langchain_when_langgraph_flag_is_false(self) -> None:
        self.patch_settings(ai_use_langgraph=False)
        provider = AIOrchestrator(build_provider("fallback"))
        self.assertEqual(provider.__class__.__name__, "AIOrchestrator")

    def test_build_orchestrator_respects_langgraph_switch(self) -> None:
        self.patch_settings(ai_use_langgraph=False)
        provider = build_provider("fallback")
        orchestrator = build_orchestrator(provider, use_langgraph=True)
        self.assertIsInstance(orchestrator, AIOrchestrator)
        self.assertNotIsInstance(orchestrator, LangGraphOrchestrator)

        self.patch_settings(ai_use_langgraph=True)
        orchestrator2 = build_orchestrator(provider, use_langgraph=True)
        self.assertIsInstance(orchestrator2, LangGraphOrchestrator)

    def test_langgraph_orchestrator_falls_back_to_langchain_on_analysis_failure(self) -> None:
        class FlakyProvider(BaseLLMProvider):
            provider_name = "flaky"

            def __init__(self) -> None:
                self._called = False

            def expand_queries(self, keyword: Keyword, base_query: str) -> list[str]:
                return [base_query]

            def analyze(self, hotspot: Hotspot, keyword: Keyword | None) -> LLMResult:
                if not self._called:
                    self._called = True
                    raise RuntimeError("transient fail")
                return LLMResult(
                    is_real=True,
                    relevance_score=88,
                    relevance_reason="recovered",
                    keyword_mentioned=True,
                    importance="high",
                    summary="AI recovered analysis",
                    model_name="flaky",
                    raw_response={"provider": "flaky"},
                    used_fallback=False,
                    prompt_name="analysis",
                    provider="flaky",
                )

        provider = FlakyProvider()
        orchestrator = LangGraphOrchestrator(provider)
        keyword = Keyword(id=1, keyword="AI", query_template=None, enabled=True, priority=0)
        hotspot = Hotspot(
            id=1,
            title="AI 热点",
            url="https://example.com/hotspot",
            source_id=1,
            keyword_id=1,
            snippet="热点摘要",
            raw_payload={},
        )

        result, decision = orchestrator.analyze(hotspot, keyword)

        self.assertTrue(result.used_fallback is False)
        self.assertEqual(decision.decision.get("path"), "langchain")
        self.assertTrue(decision.decision.get("langgraph_fallback"))
        self.assertEqual(decision.decision.get("path_from"), "langgraph")

    def test_hotness_scoring_applies_truth_penalty_and_bounds(self) -> None:
        source = Source(id=1, name="rss", source_type="rss", enabled=True, config={"source_strength": 80})
        hotspot = Hotspot(
            id=1,
            title="AI 热点",
            url="https://example.com/hotspot",
            source_id=1,
            keyword_id=1,
            snippet="热点摘要",
            published_at=datetime(2026, 5, 24, tzinfo=timezone.utc),
            raw_payload={},
        )
        hotspot.source = source

        raw = SimpleNamespace(relevance_score=88.0, keyword_mentioned=True)
        decision = compute_hotness_score(hotspot=hotspot, analysis=raw)
        no_penalty = decision.score

        penalty_result = compute_hotness_score(hotspot=hotspot, analysis=raw, trust_penalty=30.0)

        self.assertTrue(0 <= no_penalty <= settings.hotness_max_score)
        self.assertTrue(0 <= penalty_result.score <= no_penalty)
        self.assertLess(penalty_result.score, no_penalty)

    def test_source_selector_marks_success_and_failure(self) -> None:
        source = Source(id=1, name="rss", source_type="rss", enabled=True, config={})
        self.assertNotIn("health", source.config)

        mark_source_success(source)
        health = source.config.get("health")
        self.assertEqual(health.get("status"), "healthy")
        self.assertEqual(health.get("consecutive_failures"), 0)
        self.assertEqual(health.get("success_count"), 1)

        mark_source_failure(source, reason="err1")
        health = source.config["health"]
        self.assertEqual(health.get("status"), "recovering")
        self.assertEqual(health.get("consecutive_failures"), 1)
        self.assertEqual(health.get("failure_count"), 1)

        for _ in range(settings.source_failure_threshold):
            mark_source_failure(source, reason="err2")
        health = source.config["health"]
        self.assertEqual(health.get("status"), "degraded")
        self.assertGreaterEqual(int(health.get("consecutive_failures") or 0), settings.source_failure_threshold)

        sorted_sources = select_sources([source])
        self.assertEqual(len(sorted_sources), 1)
        self.assertEqual(sorted_sources[0].id, 1)

    def test_source_selector_prioritizes_healthy_over_degraded(self) -> None:
        healthy = Source(
            id=1,
            name="A",
            source_type="rss",
            enabled=True,
            config={"weight": 10, "health": {"status": "healthy", "consecutive_failures": 0}},
        )
        degraded = Source(
            id=2,
            name="B",
            source_type="rss",
            enabled=True,
            config={"weight": 100, "health": {"status": "degraded", "consecutive_failures": 5}},
        )
        recovering = Source(
            id=3,
            name="C",
            source_type="rss",
            enabled=True,
            config={"weight": 20, "health": {"status": "recovering", "consecutive_failures": 2}},
        )

        ordered = select_sources([degraded, healthy, recovering])
        self.assertEqual([source.id for source in ordered], [1, 3, 2])

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
        from server.app.services import rss as rss_service

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
