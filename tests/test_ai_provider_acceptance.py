from __future__ import annotations

import unittest
from unittest.mock import patch

from server.app.core.settings import settings
from server.app.services.ai_analysis import AnalysisResult


class AiProviderAcceptanceScriptTests(unittest.TestCase):
    def setUp(self) -> None:
        self._original_settings: dict[str, object] = {}

    def tearDown(self) -> None:
        for key, value in self._original_settings.items():
            setattr(settings, key, value)

    def patch_settings(self, **values: object) -> None:
        for key, value in values.items():
            if key not in self._original_settings:
                self._original_settings[key] = getattr(settings, key)
            setattr(settings, key, value)

    def test_missing_credentials_returns_actionable_result_without_calling_provider(self) -> None:
        from scripts.verify_ai_provider import run_provider_acceptance

        self.patch_settings(ai_provider="openai", openai_api_key=None, openai_model=None)

        with patch("scripts.verify_ai_provider.analyze_hotspot") as analyze_spy:
            result = run_provider_acceptance()

        self.assertFalse(result.ok)
        self.assertEqual(result.status, "missing_credentials")
        self.assertEqual(result.provider, "openai")
        self.assertIn("OPENAI_API_KEY", result.required_env)
        self.assertIn("OPENAI_MODEL", result.required_env)
        analyze_spy.assert_not_called()

    def test_successful_real_provider_result_requires_trace_and_decision(self) -> None:
        from scripts.verify_ai_provider import run_provider_acceptance

        self.patch_settings(ai_provider="openai", openai_api_key="test-key", openai_model="gpt-test")
        provider_result = AnalysisResult(
            is_real=True,
            relevance_score=88.0,
            relevance_reason="真实 provider 返回",
            keyword_mentioned=True,
            importance="high",
            summary="AI provider 验收摘要",
            model_name="gpt-test",
            raw_response={},
            ai_orchestrator_decision="langchain",
            trace_id="trace-123",
            provider_trace=[{"event": "analyze_success", "trace_id": "trace-123"}],
            provider="openai",
        )

        with patch("scripts.verify_ai_provider.analyze_hotspot", return_value=provider_result) as analyze_spy:
            result = run_provider_acceptance()

        self.assertTrue(result.ok)
        self.assertEqual(result.status, "passed")
        self.assertEqual(result.provider, "openai")
        self.assertEqual(result.trace_id, "trace-123")
        self.assertEqual(result.ai_orchestrator_decision, "langchain")
        analyze_spy.assert_called_once()

    def test_provider_response_without_trace_fails_validation(self) -> None:
        from scripts.verify_ai_provider import run_provider_acceptance

        self.patch_settings(ai_provider="openai", openai_api_key="test-key", openai_model="gpt-test")
        provider_result = AnalysisResult(
            is_real=True,
            relevance_score=88.0,
            relevance_reason="缺少 trace",
            keyword_mentioned=True,
            importance="high",
            summary="AI provider 验收摘要",
            model_name="gpt-test",
            raw_response={},
            ai_orchestrator_decision="langchain",
            trace_id=None,
            provider_trace=[],
            provider="openai",
        )

        with patch("scripts.verify_ai_provider.analyze_hotspot", return_value=provider_result):
            result = run_provider_acceptance()

        self.assertFalse(result.ok)
        self.assertEqual(result.status, "validation_failed")
        self.assertIn("trace_id", result.validation_errors)
        self.assertIn("provider_trace", result.validation_errors)


if __name__ == "__main__":
    unittest.main()
