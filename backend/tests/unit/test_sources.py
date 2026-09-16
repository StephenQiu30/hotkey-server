import json
from hashlib import sha256

import httpx
import pytest
from pydantic import ValidationError

from sources.adapters.bilibili import Bilibili
from sources.adapters.bluesky import Bluesky
from sources.execution import PublicCollectionFetcher
from sources.schemas import (
    BilibiliCommentsInput,
    BilibiliPostInput,
    BilibiliRepliesInput,
    BilibiliSearchInput,
    CollectionPageInput,
    QueryPreviewInput,
    SearchInput,
    SocialObject,
    ThreadInput,
)
from sources.services import SourceService

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


def bilibili_adapter(handler):
    return Bilibili(httpx.Client(transport=httpx.MockTransport(handler)))


def test_priority_source_catalog_separates_support_rights_and_pipeline():
    sources = {source.id: source for source in SourceService().catalog()}
    assert set(sources) == {"x", "bilibili", "weibo", "xiaohongshu", "douyin", "bluesky"}
    assert all(
        operation.pipeline == "not_connected" and not operation.eligible_for_collection
        for source in sources.values()
        for operation in source.operations
    )
    assert all(not hasattr(source, "pipeline") for source in sources.values())
    bilibili = {operation.operation: operation for operation in sources["bilibili"].operations}
    assert set(bilibili) == {"search_posts", "fetch_post", "list_comments", "list_replies"}
    assert all(operation.support == "supported" for operation in bilibili.values())
    assert all(operation.rights == "unknown" for operation in bilibili.values())
    x_search = next(
        operation for operation in sources["x"].operations if operation.operation == "search_posts"
    )
    assert x_search.support == "authorization_required"
    assert x_search.content_purchase_cost == 0
    assert x_search.evidence_ref.endswith("EV-007-001-source-poc.json")


def test_operation_admission_requires_rights_pipeline_and_implemented_consumer():
    rights_only = SourceService(rights_allowed={"bilibili.search_posts"})
    search = next(
        operation
        for operation in rights_only.catalog()[1].operations
        if operation.operation == "search_posts"
    )
    assert search.rights == "allowed"
    assert search.pipeline == "not_connected"
    assert not search.eligible_for_collection

    admitted = SourceService(
        rights_allowed={
            "bilibili.search_posts",
            "bilibili.fetch_post",
            "bilibili.list_comments",
            "bilibili.list_replies",
        },
        pipelines_connected={
            "bilibili.search_posts",
            "bilibili.fetch_post",
            "bilibili.list_comments",
            "bilibili.list_replies",
        },
    )
    operations = {item.operation: item for item in admitted.catalog()[1].operations}
    assert operations["search_posts"].eligible_for_collection
    assert operations["search_posts"].requires_operations == ["fetch_post"]
    assert operations["search_posts"].pipeline == "connected"
    assert operations["fetch_post"].requires_operations == ["list_comments"]
    assert operations["fetch_post"].eligible_for_collection
    assert operations["list_comments"].requires_operations == ["list_replies"]
    assert operations["list_comments"].eligible_for_collection
    assert operations["list_replies"].eligible_for_collection
    assert not admitted.activation_issues(["bilibili"])
    assert admitted.request_value_is_valid("bilibili", "fetch_post", "bvid:BV1BVFWeHEaV")
    assert admitted.request_value_is_valid("bilibili", "list_comments", "aid:113")
    assert admitted.request_value_is_valid("bilibili", "list_replies", "aid:113/root:201")
    assert not admitted.request_value_is_valid("bilibili", "fetch_post", "video:113")
    assert not admitted.request_value_is_valid("bilibili", "list_replies", "113/root:201")
    assert not admitted.request_value_is_valid("bilibili", "list_replies", "aid:١/root:201")

    missing_comments = SourceService(
        rights_allowed={"bilibili.search_posts", "bilibili.fetch_post"},
        pipelines_connected={"bilibili.search_posts", "bilibili.fetch_post"},
    )
    missing_operations = {item.operation: item for item in missing_comments.catalog()[1].operations}
    assert not missing_operations["fetch_post"].eligible_for_collection
    assert not missing_operations["search_posts"].eligible_for_collection

    with pytest.raises(ValueError, match="persistent source operation is not implemented"):
        SourceService(pipelines_connected={"bluesky.fetch_post"})
    with pytest.raises(ValueError, match="unknown source operation"):
        SourceService(rights_allowed={"unknown.search_posts"})


def test_normalized_object_rejects_invalid_relationships_and_unknown_fields():
    base = {
        "provider_namespace": "video",
        "external_id": "video:1",
        "kind": "post",
        "text": "AI",
        "author_id": "author:1",
        "created_at": "2026-09-15T00:00:00Z",
        "root_id": "video:1",
    }
    with pytest.raises(ValidationError):
        SocialObject.model_validate(dict(base, root_id="video:2"))
    with pytest.raises(ValidationError):
        SocialObject.model_validate(
            dict(base, provider_namespace="comment", kind="reply", parent_id=None)
        )
    with pytest.raises(ValidationError):
        SocialObject.model_validate(dict(base, legacy_identity="ignored"))


def test_query_preview_compiles_without_network_and_exposes_rule_boundaries():
    preview = SourceService().preview(
        QueryPreviewInput.model_validate(
            {
                "query_spec": {
                    "include_any": [" AI ", "人工智能"],
                    "include_all": ["监管"],
                    "exclude": ["招聘"],
                    "aliases": ["生成式AI", "AI"],
                },
                "source_ids": ["bilibili", "xiaohongshu", "bilibili"],
                "since": "2026-09-01T08:00:00+08:00",
                "until": "2026-09-08T00:00:00Z",
            }
        )
    )
    assert preview.network_accessed is False
    assert preview.since.isoformat() == "2026-09-01T00:00:00+00:00"
    assert [source.source for source in preview.sources] == ["bilibili", "xiaohongshu"]
    bilibili, xiaohongshu = preview.sources
    assert bilibili.queries == ["AI", "人工智能", "生成式AI"]
    assert {rule.rule: rule.mode for rule in bilibili.rules} == {
        "include_any": "native",
        "include_all": "local_filter",
        "exclude": "local_filter",
        "aliases": "native",
    }
    assert xiaohongshu.queries == []
    assert all(rule.mode == "unsupported" for rule in xiaohongshu.rules)
    assert preview.estimated_requests == 15
    assert all(not source.pipeline_connected for source in preview.sources)

    admitted = SourceService(
        rights_allowed={
            "bilibili.search_posts",
            "bilibili.fetch_post",
            "bilibili.list_comments",
            "bilibili.list_replies",
        },
        pipelines_connected={
            "bilibili.search_posts",
            "bilibili.fetch_post",
            "bilibili.list_comments",
            "bilibili.list_replies",
        },
    ).preview(
        QueryPreviewInput.model_validate(
            {
                "query_spec": {"include_any": ["AI"]},
                "source_ids": ["bilibili"],
                "since": "2026-09-01T00:00:00Z",
                "until": "2026-09-08T00:00:00Z",
            }
        )
    )
    assert admitted.sources[0].pipeline_connected


def test_window_validation():
    with pytest.raises(ValidationError):
        SearchInput(keyword="x", since="2026-09-09T00:00:00Z", until="2026-09-01T00:00:00Z")
    with pytest.raises(ValidationError):
        SearchInput(keyword="x", since="2026-09-01T00:00:00", until="2026-09-09T00:00:00Z")
    with pytest.raises(ValidationError):
        ThreadInput(uri="http://127.0.0.1/private")
    with pytest.raises(ValidationError):
        CollectionPageInput(
            source="bilibili",
            operation="search_posts",
            request_value="x",
            since="2026-09-09T00:00:00Z",
            until="2026-09-01T00:00:00Z",
        )


def test_public_fetcher_rejects_invalid_bilibili_post_reference_before_network():
    with pytest.raises(ValidationError, match="fetch_post requires a bvid reference"):
        PublicCollectionFetcher().fetch(
            CollectionPageInput(
                source="bilibili",
                operation="fetch_post",
                request_value="video:113",
                since="2026-09-01T00:00:00Z",
                until="2026-09-09T00:00:00Z",
            )
        )


def test_public_fetcher_rejects_invalid_bilibili_comment_reference_before_network():
    with pytest.raises(ValidationError, match="list_comments requires an aid reference"):
        PublicCollectionFetcher().fetch(
            CollectionPageInput(
                source="bilibili",
                operation="list_comments",
                request_value="aid:0",
                since="2026-09-01T00:00:00Z",
                until="2026-09-09T00:00:00Z",
                limit=20,
            )
        )


def test_public_fetcher_rejects_invalid_bilibili_reply_reference_before_network():
    with pytest.raises(ValidationError, match="list_replies requires an aid and root reference"):
        PublicCollectionFetcher().fetch(
            CollectionPageInput(
                source="bilibili",
                operation="list_replies",
                request_value="aid:113/root:0",
                since="2026-09-01T00:00:00Z",
                until="2026-09-09T00:00:00Z",
                limit=20,
            )
        )


def test_collection_page_cursor_is_numeric_and_only_allowed_for_comment_operations():
    comments = CollectionPageInput(
        source="bilibili",
        operation="list_comments",
        request_value="aid:113",
        cursor="1",
        since="2026-09-01T00:00:00Z",
        until="2026-09-09T00:00:00Z",
        limit=20,
    )
    replies = CollectionPageInput(
        source="bilibili",
        operation="list_replies",
        request_value="aid:113/root:441",
        cursor="2",
        since="2026-09-01T00:00:00Z",
        until="2026-09-09T00:00:00Z",
        limit=20,
    )
    assert comments.cursor == "1" and replies.cursor == "2"
    with pytest.raises(ValidationError, match="cursor is only supported"):
        CollectionPageInput(
            source="bilibili",
            operation="search_posts",
            request_value="AI",
            cursor="1",
            since="2026-09-01T00:00:00Z",
            until="2026-09-09T00:00:00Z",
        )
    with pytest.raises(ValidationError):
        CollectionPageInput(
            source="bilibili",
            operation="list_comments",
            request_value="aid:113",
            cursor="next",
            since="2026-09-01T00:00:00Z",
            until="2026-09-09T00:00:00Z",
            limit=20,
        )


def test_public_fetcher_maps_checkpoint_cursor_to_bilibili_comment_pages(monkeypatch):
    seen = []

    class FakeBilibili:
        def __init__(self, client):
            self.client = client

        def comments_page(self, data):
            seen.append(data)
            return "comments"

        def replies_page(self, data):
            seen.append(data)
            return "replies"

    monkeypatch.setattr("sources.execution.Bilibili", FakeBilibili)
    fetcher = PublicCollectionFetcher()
    assert (
        fetcher.fetch(
            CollectionPageInput(
                source="bilibili",
                operation="list_comments",
                request_value="aid:113",
                cursor="7",
                since="2026-09-01T00:00:00Z",
                until="2026-09-09T00:00:00Z",
                limit=20,
            )
        )
        == "comments"
    )
    assert (
        fetcher.fetch(
            CollectionPageInput(
                source="bilibili",
                operation="list_replies",
                request_value="aid:113/root:441",
                cursor="3",
                since="2026-09-01T00:00:00Z",
                until="2026-09-09T00:00:00Z",
                limit=20,
            )
        )
        == "replies"
    )
    assert seen[0].cursor == 7
    assert seen[1].page == 3


def test_last_page_items_and_unknown_count_survive():
    def handler(request):
        assert request.url.host == "public.api.bsky.app"
        assert request.url.params["sort"] == "latest"
        return httpx.Response(200, json={"posts": [post()]})

    page = adapter(handler).search_page(search())
    result = page.result
    assert result.status == "ok" and len(result.items) == 1
    assert result.cursor is None and result.total is None
    assert result.coverage == "unknown"
    assert len(result.response_sha256) == 64
    assert page.payload is not None and page.media_type == "application/json"
    assert len(page.request_fingerprint) == 64 and page.page_key.startswith("search:")


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


def test_bilibili_bounded_search_post_comments_and_replies():
    def handler(request):
        if request.url.host == "search.bilibili.com":
            assert request.url.params["keyword"] == "人工智能"
            return httpx.Response(
                200,
                content=b'<script>res:[{bvid:"BV1BVFWeHEaV"},{bvid:"BV1BVFWeHEaV"}]</script>',
                headers={"Content-Type": "text/html"},
            )
        if request.url.path == "/x/web-interface/view":
            return httpx.Response(
                200,
                json={
                    "code": 0,
                    "data": {
                        "aid": 113,
                        "bvid": "BV1BVFWeHEaV",
                        "title": "synthetic video",
                        "desc": "synthetic description",
                        "pubdate": 1789401600,
                        "owner": {"mid": 7},
                        "stat": {"reply": 2},
                    },
                },
            )
        if request.url.path == "/x/v2/reply/main":
            assert request.url.params["next"] == "0"
            return httpx.Response(
                200,
                json={
                    "code": 0,
                    "data": {
                        "cursor": {"next": 1, "is_end": False},
                        "replies": [
                            {
                                "rpid": 201,
                                "root": 0,
                                "parent": 0,
                                "ctime": 1789401700,
                                "rcount": 1,
                                "member": {"mid": "8"},
                                "content": {"message": "synthetic root comment"},
                            }
                        ],
                    },
                },
            )
        assert request.url.path == "/x/v2/reply/reply"
        assert request.url.params["root"] == "201"
        return httpx.Response(
            200,
            json={
                "code": 0,
                "data": {
                    "page": {"num": 1, "size": 20, "count": 1},
                    "replies": [
                        {
                            "rpid": 202,
                            "root": 201,
                            "parent": 201,
                            "ctime": 1789401800,
                            "rcount": 0,
                            "member": {"mid": "9"},
                            "content": {"message": "synthetic reply"},
                        }
                    ],
                },
            },
        )

    source = bilibili_adapter(handler)
    search_result = source.search(BilibiliSearchInput(keyword="人工智能", limit=20))
    assert search_result.status == "ok"
    assert [reference.external_id for reference in search_result.references] == [
        "bvid:BV1BVFWeHEaV"
    ]
    post_page = source.post_page(BilibiliPostInput(bvid="BV1BVFWeHEaV"))
    post_result = post_page.result
    assert post_result.items[0].external_id == "video:113"
    assert post_result.items[0].reply_count == 2
    assert [reference.external_id for reference in post_result.references] == ["aid:113"]
    assert post_page.payload is not None
    assert post_result.response_sha256 == sha256(post_page.payload).hexdigest()
    assert post_page.page_key.startswith("post:")
    comments_page = source.comments_page(BilibiliCommentsInput(aid=113, cursor=0, limit=20))
    comments = comments_page.result
    assert comments.cursor == "1"
    assert comments.items[0].kind == "comment"
    assert [reference.external_id for reference in comments.references] == ["aid:113/root:201"]
    assert comments_page.payload is not None
    assert comments.response_sha256 == sha256(comments_page.payload).hexdigest()
    assert comments_page.page_key.startswith("comments:")
    assert comments.items[0].root_id == "video:113"
    replies_page = source.replies_page(BilibiliRepliesInput(aid=113, root_id=201, page=1, limit=20))
    replies = replies_page.result
    assert replies.items[0].kind == "reply"
    assert replies.items[0].parent_id == "comment:201"
    assert replies.cursor is None
    assert replies_page.payload is not None
    assert replies.response_sha256 == sha256(replies_page.payload).hexdigest()
    assert replies_page.page_key.startswith("replies:")


def test_bilibili_post_without_replies_does_not_create_comment_reference():
    def handler(request):
        assert request.url.path == "/x/web-interface/view"
        return httpx.Response(
            200,
            json={
                "code": 0,
                "data": {
                    "aid": 113,
                    "bvid": "BV1BVFWeHEaV",
                    "title": "synthetic video",
                    "desc": "synthetic description",
                    "pubdate": 1789401600,
                    "owner": {"mid": 7},
                    "stat": {"reply": 0},
                },
            },
        )

    result = bilibili_adapter(handler).post(BilibiliPostInput(bvid="BV1BVFWeHEaV"))
    assert result.status == "ok"
    assert result.items[0].reply_count == 0
    assert result.references == []


def test_bilibili_root_comment_without_replies_does_not_create_reply_reference():
    def handler(request):
        assert request.url.path == "/x/v2/reply/main"
        return httpx.Response(
            200,
            json={
                "code": 0,
                "data": {
                    "cursor": {"next": 0, "is_end": True},
                    "replies": [
                        {
                            "rpid": 201,
                            "root": 0,
                            "parent": 0,
                            "ctime": 1789401700,
                            "rcount": 0,
                            "member": {"mid": "8"},
                            "content": {"message": "synthetic root comment"},
                        }
                    ],
                },
            },
        )

    result = bilibili_adapter(handler).comments(BilibiliCommentsInput(aid=113))
    assert result.status == "ok"
    assert result.items[0].reply_count == 0
    assert result.references == []


def test_bilibili_challenge_and_schema_drift_are_not_empty_success():
    challenged = bilibili_adapter(lambda _: httpx.Response(412, json={"code": -412})).search(
        BilibiliSearchInput(keyword="人工智能")
    )
    assert challenged.status == "failed" and challenged.code == "access_denied"
    changed = bilibili_adapter(
        lambda _: httpx.Response(200, content=b"<html>changed</html>")
    ).search(BilibiliSearchInput(keyword="人工智能"))
    assert changed.status == "failed" and changed.code == "schema_changed"


def test_probe_validation_needs_no_runtime_secrets(monkeypatch, capsys):
    from cli.commands import main

    monkeypatch.delenv("HOTKEY_DATABASE_URL", raising=False)
    monkeypatch.delenv("HOTKEY_BROKER_URL", raising=False)
    monkeypatch.setattr(
        "sys.argv", ["cli", "source-probe", "bluesky", "thread", "--uri", "private-input"]
    )
    with pytest.raises(SystemExit) as exc:
        main()
    assert exc.value.code == 2
    output = json.loads(capsys.readouterr().out)
    assert output == {"status": "failed", "code": "validation_failed"}
