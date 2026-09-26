from datetime import UTC, datetime, timedelta
from types import SimpleNamespace
from unittest.mock import MagicMock
from uuid import uuid4

import httpx

from connections.services import AppliedHotlistPreset
from content.hotlist import _post_payload, match_hotlist_topics, rank_change
from core.config import Settings
from main import create_app
from monitors.services import ActiveHotlistTopic, normalize_monitor_rules
from reports.services import ReportService
from sources.adapters.rsshub_hotlist import RsshubHotlistAdapter
from sources.contracts import HotlistEntry, SourceCapability
from worker import scheduler
from worker.scheduler import hotlist_operation_id

FEED = b"""<rss version="2.0"><channel><title>Hot</title>
<item><title>First</title><link>https://example.com/first</link><description>One</description></item>
<item><title>Second</title><link>https://example.com/second</link>
<pubDate>Fri, 25 Sep 2026 08:00:00 GMT</pubDate></item>
</channel></rss>"""


def test_rsshub_hotlist_preserves_rank_and_missing_publication_time() -> None:
    adapter = RsshubHotlistAdapter(
        source_key="hotlist_weibo",
        feed_url="http://127.0.0.1:1200/weibo/search/hot",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _: True,
        transport=httpx.MockTransport(lambda _: httpx.Response(200, content=FEED)),
    )
    page = adapter.fetch_hotlist()
    assert page.capability is SourceCapability.HOTLIST
    assert [(item.rank, item.title) for item in page.items] == [(1, "First"), (2, "Second")]
    assert page.items[0].published_at is None
    assert page.items[1].published_at == datetime(2026, 9, 25, 8, tzinfo=UTC)
    assert page.request_count == 1


def test_rsshub_hotlist_rejects_redirect_outside_local_allowlist() -> None:
    requested: list[str] = []

    def respond(request: httpx.Request) -> httpx.Response:
        requested.append(str(request.url))
        return httpx.Response(302, headers={"location": "https://example.com/feed"})

    adapter = RsshubHotlistAdapter(
        source_key="hotlist_weibo",
        feed_url="http://127.0.0.1:1200/weibo/search/hot",
        allowed_hosts=frozenset({"127.0.0.1"}),
        before_request=lambda _: True,
        transport=httpx.MockTransport(respond),
    )
    page = adapter.fetch_hotlist()
    assert page.stop_reason.value == "access_denied"
    assert len(requested) == 1


def test_hotlist_content_uses_observation_time_even_with_feed_pubdate() -> None:
    entry = HotlistEntry(
        rank=1,
        title="First",
        url="https://example.com/first",
        published_at=datetime(2026, 9, 25, 8, tzinfo=UTC),
    )
    assert _post_payload(entry, "hotlist_weibo")["published_at"] is None


def test_rank_change_covers_new_up_down_and_same() -> None:
    assert rank_change(1, None) == "new"
    assert rank_change(1, 2) == "up"
    assert rank_change(3, 2) == "down"
    assert rank_change(2, 2) == "same"


def test_hotlist_operation_id_is_stable_within_bucket() -> None:
    owner_id = uuid4()
    now = datetime(2026, 9, 26, 8, 2, tzinfo=UTC)
    assert hotlist_operation_id(owner_id, "hotlist_weibo", now, 1800) == hotlist_operation_id(
        owner_id, "hotlist_weibo", now + timedelta(minutes=20), 1800
    )
    assert hotlist_operation_id(owner_id, "hotlist_weibo", now, 1800) != hotlist_operation_id(
        owner_id, "hotlist_weibo", now + timedelta(minutes=30), 1800
    )


def test_hotlist_matches_each_active_topic_using_title_and_summary() -> None:
    owner_id = uuid4()
    entry = HotlistEntry(
        rank=1,
        title="新款手机发布",
        url="https://example.com/1",
        summary="人工智能功能升级",
        published_at=None,
    )
    topics = (
        ActiveHotlistTopic(
            owner_id=owner_id,
            topic_id=uuid4(),
            name="手机 AI",
            rules=normalize_monitor_rules(
                match_any=("手机",),
                match_all=("人工智能",),
                exclude=(),
            ),
        ),
        ActiveHotlistTopic(
            owner_id=owner_id,
            topic_id=uuid4(),
            name="排除发布",
            rules=normalize_monitor_rules(
                match_any=("手机",),
                match_all=(),
                exclude=("发布",),
            ),
        ),
    )
    assert match_hotlist_topics(entry, topics) == ("手机 AI",)


def test_repeated_hotlist_scan_does_not_accept_same_operation(monkeypatch: object) -> None:
    owner_id = uuid4()
    preset = AppliedHotlistPreset(
        owner_id=owner_id,
        source_key="hotlist_weibo",
        connection_id=uuid4(),
        connection_version=1,
    )
    accepted: set[object] = set()

    class FakeSession:
        def in_transaction(self) -> bool:
            return True

    class FakeJobs:
        def __init__(self, _session: object, *, clock: object) -> None:
            pass

        def operation_exists_in_transaction(
            self, *, owner_id: object, kind: str, operation_id: object
        ) -> bool:
            return operation_id in accepted

        def accept_in_transaction(self, *, owner_id: object, command: object) -> None:
            accepted.add(command.operation_id)

    monkeypatch.setattr(scheduler, "JobService", FakeJobs)
    monkeypatch.setattr(
        scheduler, "list_applied_hotlist_presets_in_transaction", lambda _: (preset,)
    )
    monkeypatch.setattr(
        scheduler, "get_settings", lambda: SimpleNamespace(hotlist_interval_seconds=1800)
    )
    now = datetime(2026, 9, 26, 8, 2, tzinfo=UTC)
    session = FakeSession()
    assert scheduler.enqueue_due_hotlists_in_transaction(session, now) == 1
    assert scheduler.enqueue_due_hotlists_in_transaction(session, now + timedelta(minutes=20)) == 0
    assert len(accepted) == 1


def test_daily_report_selects_hotlist_discoveries_by_topic_id() -> None:
    session = MagicMock()
    session.execute.return_value.mappings.return_value = []
    owner_id = uuid4()
    topic_id = uuid4()
    start = datetime(2026, 9, 25, tzinfo=UTC)
    assert (
        ReportService(session)._load_posts(
            owner_id=owner_id,
            topic_id=topic_id,
            window_start=start,
            window_end=start + timedelta(days=1),
            cutoff_at=start + timedelta(days=2),
        )
        == ()
    )
    statement = session.execute.call_args.args[0]
    parameters = session.execute.call_args.args[1]
    assert "discovery_job.kind = 'source.hotlist'" in statement.text
    assert "entry.matched_topic_ids ? :topic_id_text" in statement.text
    assert parameters["topic_id_text"] == str(topic_id)


def test_hotlist_api_openapi_declares_auth_and_result_models() -> None:
    app = create_app(
        Settings(environment="test", database_url="postgresql+psycopg://test:test@localhost/test")
    )
    paths = app.openapi()["paths"]
    sources = paths["/api/hotlists/sources"]["get"]
    latest = paths["/api/hotlists/{source_key}"]["get"]
    assert sources["operationId"] == "listHotlistSources"
    assert latest["operationId"] == "getHotlistSnapshot"
    assert sources["security"] == [{"SessionCookie": []}]
    assert latest["security"] == [{"SessionCookie": []}]
    assert {"200", "401", "422", "500"} <= set(sources["responses"])
    assert {"200", "401", "404", "422", "500"} <= set(latest["responses"])
