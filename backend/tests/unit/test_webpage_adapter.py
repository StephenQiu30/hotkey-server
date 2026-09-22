from __future__ import annotations

import hashlib
import json
from datetime import UTC, datetime
from types import SimpleNamespace

import httpx
import pytest
from pydantic import ValidationError
from typer.testing import CliRunner

import cli.commands as cli_commands
from cli.commands import app as cli_app
from jobs.schemas import CollectionJobInput
from sources.adapters.firecrawl import FirecrawlAdapter
from sources.adapters.web_targets import normalize_web_url
from sources.contracts import (
    SourceCapability,
    SourceDocument,
    SourceStopReason,
    WebPageRequest,
    WebPageResult,
)


def _response(
    *,
    markdown: str = "# Example\n\nUseful text.",
    status_code: int = 200,
    final_url: str = "https://example.com/final",
    title: str = "Example",
) -> dict[str, object]:
    return {
        "success": True,
        "data": {
            "markdown": markdown,
            "metadata": {
                "url": final_url,
                "sourceURL": "https://example.com/start",
                "statusCode": status_code,
                "title": title,
                "publishedTime": "2026-09-21T01:02:03Z",
            },
        },
    }


def _adapter(handler: httpx.MockTransport, **kwargs: object) -> FirecrawlAdapter:
    return FirecrawlAdapter(
        base_url="http://127.0.0.1:3002",
        enabled=True,
        allowed_hosts=frozenset({"example.com"}),
        transport=handler,
        clock=lambda: datetime(2026, 9, 22, 8, 0, tzinfo=UTC),
        **kwargs,
    )


def test_webpage_contract_and_target_normalization_reject_unsafe_urls() -> None:
    request = WebPageRequest(url="https://Example.COM:443/path?q=1#fragment")
    assert request.capability is SourceCapability.PAGE_CONTENT
    assert request.timeout_seconds == 20
    assert request.max_content_characters == 100_000
    assert normalize_web_url(request.url, allowed_hosts=frozenset({"example.com"})) == (
        "https://example.com/path?q=1"
    )

    for url in (
        "ftp://example.com/file",
        "https://user:pass@example.com/file",
        "https://example.com:80/file",
        "https://example.com:8443/file",
        "https://127.0.0.1/file",
        "https://8.8.8.8/file",
        "https://other.example/file",
    ):
        with pytest.raises((ValidationError, ValueError)):
            normalize_web_url(url, allowed_hosts=frozenset({"example.com", "127.0.0.1"}))


def test_page_content_does_not_open_the_existing_monitor_job_api() -> None:
    with pytest.raises(ValidationError):
        CollectionJobInput.model_validate(
            {
                "operation_id": "00000000-0000-0000-0000-000000000047",
                "kind": "monitor.collect",
                "observation": {
                    "configuration_ref": "monitor-config-1",
                    "configuration_version": 1,
                    "source_key": "web",
                    "source_capability": "page_content",
                },
                "scheduled_for_at": None,
                "scope": {"url_ref": "not-a-url"},
            }
        )

    with pytest.raises(ValidationError):
        WebPageResult(
            document=None,
            stop_reason=SourceStopReason.ACCESS_DENIED,
            target_status_code=403,
            collector_call_count=0,
            target_request_count=None,
        )


def test_firecrawl_adapter_sends_a_fixed_bounded_request_and_maps_document() -> None:
    def handle(request: httpx.Request) -> httpx.Response:
        assert request.url == "http://127.0.0.1:3002/v2/scrape"
        assert request.headers.get("authorization") is None
        assert request.headers.get("cookie") is None
        assert json.loads(request.content) == {
            "url": "https://example.com/start",
            "formats": ["markdown"],
            "onlyMainContent": True,
            "maxAge": 0,
            "storeInCache": False,
            "timeout": 20_000,
        }
        return httpx.Response(200, json=_response(), request=request)

    with _adapter(httpx.MockTransport(handle)) as adapter:
        result = adapter.fetch_document(WebPageRequest(url="https://example.com/start"))

    assert result.stop_reason is None
    assert result.target_status_code == 200
    assert result.collector_call_count == 1
    assert result.target_request_count is None
    assert result.document is not None
    assert result.document.request_url == "https://example.com/start"
    assert result.document.final_url == "https://example.com/final"
    assert result.document.title == "Example"
    assert result.document.text == "# Example\n\nUseful text."
    assert result.document.text_scope == "full"
    assert result.document.published_at == datetime(2026, 9, 21, 1, 2, 3, tzinfo=UTC)
    assert result.document.observed_at == datetime(2026, 9, 22, 8, 0, tzinfo=UTC)
    assert (
        result.document.content_fingerprint
        == hashlib.sha256(result.document.text.encode()).hexdigest()
    )
    assert result.document.extractor_version == "firecrawl/2.11.162"


@pytest.mark.parametrize(
    ("payload", "reason"),
    [
        ({"success": False, "error": "upstream details"}, SourceStopReason.PROTOCOL_ERROR),
        (_response(status_code=401), SourceStopReason.AUTHENTICATION_REQUIRED),
        (_response(status_code=403), SourceStopReason.ACCESS_DENIED),
        (_response(status_code=404), SourceStopReason.NOT_FOUND),
        (_response(status_code=429), SourceStopReason.RATE_LIMITED),
        (_response(status_code=503), SourceStopReason.UPSTREAM_ERROR),
        (_response(markdown=""), SourceStopReason.SOURCE_EMPTY),
        (
            _response(final_url="https://example.com/login"),
            SourceStopReason.AUTHENTICATION_REQUIRED,
        ),
        (
            _response(final_url="https://other.example/final"),
            SourceStopReason.ACCESS_DENIED,
        ),
        (_response(title="Verify you are human"), SourceStopReason.AUTHENTICATION_REQUIRED),
    ],
)
def test_firecrawl_adapter_rejects_false_success_and_unusable_pages(
    payload: dict[str, object], reason: SourceStopReason
) -> None:
    def handle(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json=payload, request=request)

    with _adapter(httpx.MockTransport(handle)) as adapter:
        result = adapter.fetch_document(WebPageRequest(url="https://example.com/start"))

    assert result.document is None
    assert result.stop_reason is reason
    assert result.collector_call_count == 1


def test_firecrawl_adapter_bounds_response_and_content_separately() -> None:
    body = _response(markdown="0123456789")

    def handle(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json=body, request=request)

    with _adapter(httpx.MockTransport(handle)) as adapter:
        result = adapter.fetch_document(
            WebPageRequest(url="https://example.com/start", max_content_characters=5)
        )
    assert result.document is not None
    assert result.document.text == "01234"
    assert result.document.text_scope == "truncated"

    oversized = json.dumps(body).encode()
    with _adapter(
        httpx.MockTransport(
            lambda request: httpx.Response(200, content=oversized, request=request)
        ),
        max_response_bytes=len(oversized) - 1,
    ) as adapter:
        rejected = adapter.fetch_document(WebPageRequest(url="https://example.com/start"))
    assert rejected.document is None
    assert rejected.stop_reason is SourceStopReason.BUDGET_EXHAUSTED


def test_firecrawl_adapter_maps_transport_and_service_failures_without_raw_details() -> None:
    cases = (
        (
            httpx.MockTransport(
                lambda request: (_ for _ in ()).throw(
                    httpx.ReadTimeout("private timeout detail", request=request)
                )
            ),
            SourceStopReason.UPSTREAM_ERROR,
        ),
        (
            httpx.MockTransport(
                lambda request: httpx.Response(429, text="private rate detail", request=request)
            ),
            SourceStopReason.RATE_LIMITED,
        ),
        (
            httpx.MockTransport(
                lambda request: httpx.Response(200, text="not-json", request=request)
            ),
            SourceStopReason.PROTOCOL_ERROR,
        ),
    )
    for transport, reason in cases:
        with _adapter(transport) as adapter:
            result = adapter.fetch_document(WebPageRequest(url="https://example.com/start"))
        assert result.document is None
        assert result.stop_reason is reason
        assert "private" not in repr(result)


def test_disabled_firecrawl_fails_closed_without_a_collector_call() -> None:
    called = False

    def handle(request: httpx.Request) -> httpx.Response:
        nonlocal called
        called = True
        return httpx.Response(200, json=_response(), request=request)

    with FirecrawlAdapter(
        base_url="http://127.0.0.1:3002",
        enabled=False,
        allowed_hosts=frozenset({"example.com"}),
        transport=httpx.MockTransport(handle),
    ) as adapter:
        result = adapter.fetch_document(WebPageRequest(url="https://example.com/start"))
    assert result.document is None
    assert result.stop_reason is SourceStopReason.UNSUPPORTED
    assert result.collector_call_count == 0
    assert not called


def test_webpage_probe_cli_runs_explicit_probe_without_echoing_target(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    observed: dict[str, object] = {}

    class ProbeAdapter:
        def __init__(self, **kwargs: object) -> None:
            observed["adapter"] = kwargs

        def __enter__(self) -> ProbeAdapter:
            return self

        def __exit__(self, *args: object) -> None:
            return None

        def fetch_document(self, request: WebPageRequest) -> WebPageResult:
            observed["request"] = request
            return WebPageResult(
                document=SourceDocument(
                    request_url="https://example.com/private?token=secret",
                    final_url="https://example.com/final?token=secret",
                    title="Private title",
                    text="Private body",
                    text_scope="full",
                    observed_at=datetime(2026, 9, 22, 8, 0, tzinfo=UTC),
                    published_at=None,
                    content_fingerprint=hashlib.sha256(b"Private body").hexdigest(),
                    extractor_version="firecrawl/2.11.162",
                ),
                stop_reason=None,
                target_status_code=200,
                collector_call_count=1,
                target_request_count=None,
            )

    monkeypatch.setattr(cli_commands, "FirecrawlAdapter", ProbeAdapter)
    monkeypatch.setattr(
        cli_commands,
        "get_settings",
        lambda: SimpleNamespace(
            firecrawl_base_url="http://127.0.0.1:3002",
            firecrawl_enabled=True,
            firecrawl_timeout_seconds=20,
            firecrawl_max_response_bytes=2 * 1024 * 1024,
        ),
    )

    result = CliRunner().invoke(
        cli_app,
        [
            "sources",
            "probe-webpage",
            "--url",
            "https://example.com/private?token=secret",
            "--allowed-host",
            "example.com",
        ],
    )

    assert result.exit_code == 0
    assert "status: succeeded" in result.output
    assert "characters: 12" in result.output
    assert "target requests: unknown" in result.output
    assert "example.com" not in result.output
    assert "secret" not in result.output
    assert "Private" not in result.output
    assert observed["request"] == WebPageRequest(
        url="https://example.com/private?token=secret",
        timeout_seconds=20,
    )
    assert observed["adapter"] == {
        "base_url": "http://127.0.0.1:3002",
        "enabled": True,
        "allowed_hosts": frozenset({"example.com"}),
        "max_response_bytes": 2 * 1024 * 1024,
    }


def test_webpage_probe_cli_returns_stable_failure_without_echoing_target(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    class RejectedProbeAdapter:
        def __init__(self, **kwargs: object) -> None:
            pass

        def __enter__(self) -> RejectedProbeAdapter:
            return self

        def __exit__(self, *args: object) -> None:
            return None

        def fetch_document(self, request: WebPageRequest) -> WebPageResult:
            return WebPageResult(
                document=None,
                stop_reason=SourceStopReason.ACCESS_DENIED,
                target_status_code=403,
                collector_call_count=1,
                target_request_count=None,
            )

    monkeypatch.setattr(cli_commands, "FirecrawlAdapter", RejectedProbeAdapter)
    monkeypatch.setattr(
        cli_commands,
        "get_settings",
        lambda: SimpleNamespace(
            firecrawl_base_url="http://127.0.0.1:3002",
            firecrawl_enabled=True,
            firecrawl_timeout_seconds=20,
            firecrawl_max_response_bytes=2 * 1024 * 1024,
        ),
    )

    result = CliRunner().invoke(
        cli_app,
        [
            "sources",
            "probe-webpage",
            "--url",
            "https://blocked.example/private?token=secret",
            "--allowed-host",
            "blocked.example",
        ],
    )

    assert result.exit_code == 1
    assert "status: failed; reason: access_denied" in result.output
    assert "blocked.example" not in result.output
    assert "secret" not in result.output
