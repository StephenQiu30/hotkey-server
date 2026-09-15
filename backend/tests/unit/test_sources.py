import json

import httpx
import pytest
from pydantic import ValidationError

from sources.bluesky import Bluesky
from sources.schemas import SearchInput, ThreadInput

URI = "at://did:plc:sample/app.bsky.feed.post/root"


def post(key="root", reply=None):
    record = {"text": "synthetic test", "createdAt": "2026-09-08T00:00:00Z"}
    if reply:
        record["reply"] = {"root": {"uri": URI}, "parent": {"uri": reply}}
    return {
        "uri": URI.replace("/root", "/" + key),
        "cid": "sample",
        "author": {"did": "did:plc:sample"},
        "record": record,
        "replyCount": 1,
    }


def search(**kwargs):
    return SearchInput(
        keyword="science", since="2026-09-01T00:00:00Z", until="2026-09-09T00:00:00Z", **kwargs
    )


def adapter(handler):
    return Bluesky(httpx.Client(transport=httpx.MockTransport(handler)))


def test_window_validation():
    with pytest.raises(ValidationError):
        SearchInput(keyword="x", since="2026-09-09T00:00:00Z", until="2026-09-01T00:00:00Z")
    with pytest.raises(ValidationError):
        SearchInput(keyword="x", since="2026-09-01T00:00:00", until="2026-09-09T00:00:00Z")
    with pytest.raises(ValidationError):
        ThreadInput(uri="http://127.0.0.1/private")


def test_last_page_items_and_unknown_count_survive():
    def handler(request):
        assert request.url.host == "public.api.bsky.app"
        assert request.url.params["sort"] == "latest"
        return httpx.Response(200, json={"posts": [post()]})

    result = adapter(handler).search(search())
    assert result.status == "ok" and len(result.items) == 1
    assert result.cursor is None and result.total is None
    assert result.coverage == "unknown"
    assert len(result.response_sha256) == 64


@pytest.mark.parametrize(
    ("status", "code"),
    [
        (401, "credential_required"),
        (403, "access_denied"),
        (429, "rate_limited"),
        (503, "upstream_unavailable"),
        (302, "redirect_blocked"),
    ],
)
def test_upstream_failure_is_not_empty_success(status, code):
    result = adapter(lambda _: httpx.Response(status, headers={"Retry-After": "12"})).search(
        search()
    )
    assert result.status == "failed" and result.code == code and not result.items
    if status == 429:
        assert result.retry_after_seconds == 12


def test_empty_and_schema_drift_are_distinct():
    assert (
        adapter(lambda _: httpx.Response(200, json={"posts": []})).search(search()).status
        == "empty"
    )
    result = adapter(lambda _: httpx.Response(200, json={"changed": []})).search(search())
    assert result.code == "schema_changed" and result.status == "failed"


def test_repeated_cursor_preserves_received_items_but_stops():
    result = adapter(
        lambda _: httpx.Response(200, json={"posts": [post()], "cursor": "same"})
    ).search(search(cursor="same"))
    assert result.status == "partial" and result.code == "cursor_stalled"
    assert len(result.items) == 1 and result.cursor is None


def test_thread_relationships_and_unavailable_nodes():
    payload = {
        "thread": {
            "post": post(),
            "replies": [
                {
                    "post": post("child", URI),
                    "replies": [{"post": post("reply", URI.replace("/root", "/child"))}],
                },
                {
                    "$type": "app.bsky.feed.defs#notFoundPost",
                    "uri": URI.replace("/root", "/missing"),
                    "notFound": True,
                },
            ],
        }
    }
    result = adapter(lambda _: httpx.Response(200, json=payload)).thread(ThreadInput(uri=URI))
    assert [p.kind for p in result.items] == ["post", "comment", "reply"]
    assert result.items[-1].parent_id.endswith("/child")
    assert len(result.unavailable) == 1 and result.status == "partial"


def test_thread_node_budget_is_explicit():
    payload = {"thread": {"post": post(), "replies": [{"post": post("child", URI)}]}}
    result = adapter(lambda _: httpx.Response(200, json=payload)).thread(
        ThreadInput(uri=URI, max_nodes=1)
    )
    assert len(result.items) == 1 and result.code == "node_limit" and result.status == "partial"


def test_response_size_and_timeout():
    result = adapter(lambda _: httpx.Response(200, content=b"x" * (2 * 1024 * 1024 + 1))).search(
        search()
    )
    assert result.code == "response_too_large"

    def timeout(request):
        raise httpx.ReadTimeout("sensitive query", request=request)

    result = adapter(timeout).search(search())
    assert result.code == "timeout" and "sensitive" not in result.model_dump_json()


def test_malformed_record_does_not_become_empty_success():
    result = adapter(lambda _: httpx.Response(200, json={"posts": [{"record": {}}]})).search(
        search()
    )
    assert result.code == "schema_changed"
    result = adapter(
        lambda _: httpx.Response(200, content=json.dumps({"posts": []}).encode()[:-1])
    ).search(search())
    assert result.code == "schema_changed"


def test_unavailable_root_is_failure_and_duplicate_nodes_are_bounded():
    missing = {"notFound": True, "uri": URI}
    result = adapter(lambda _: httpx.Response(200, json={"thread": missing})).thread(
        ThreadInput(uri=URI)
    )
    assert result.status == "failed" and result.code == "not_found"
    payload = {"thread": {"post": post(), "replies": [dict(missing, uri=URI + "x")] * 300}}
    result = adapter(lambda _: httpx.Response(200, json=payload)).thread(
        ThreadInput(uri=URI, max_nodes=3)
    )
    assert len(result.items) + len(result.unavailable) <= 3
    assert result.status == "partial"


def test_wrong_root_relation_fails_closed():
    child = post("child", URI)
    child["record"]["reply"]["root"]["uri"] = URI + "wrong"
    result = adapter(
        lambda _: httpx.Response(
            200, json={"thread": {"post": post(), "replies": [{"post": child}]}}
        )
    ).thread(ThreadInput(uri=URI))
    assert result.code == "schema_changed" and not result.items


def test_probe_validation_needs_no_runtime_secrets(monkeypatch, capsys):
    from cli.commands import main

    monkeypatch.delenv("HOTKEY_DATABASE_URL", raising=False)
    monkeypatch.delenv("HOTKEY_BROKER_URL", raising=False)
    monkeypatch.setattr("sys.argv", ["cli", "source-probe", "thread", "--uri", "private-input"])
    with pytest.raises(SystemExit) as exc:
        main()
    assert exc.value.code == 2
    output = json.loads(capsys.readouterr().out)
    assert output == {"status": "failed", "code": "validation_failed"}
