from __future__ import annotations

import json
import logging
import subprocess
import sys
from datetime import UTC, datetime
from pathlib import Path

import pytest

from content.discovery import KeywordDiscoveryPageCommitService
from sources.adapters.mediacrawler import MediaCrawlerAdapter, _post
from sources.contracts import (
    CommentsRequest,
    SearchRequest,
    SourceComment,
    SourcePageState,
    SourcePost,
    SourceStopReason,
)


def _fake_crawler(root: Path, script: str) -> Path:
    crawler = root / "crawler"
    executable = crawler / ".venv" / "bin" / "python"
    executable.parent.mkdir(parents=True)
    executable.symlink_to(sys.executable)
    (crawler / "main.py").write_text(script, encoding="utf-8")
    return crawler


def _adapter(
    root: Path,
    crawler: Path,
    *,
    max_seconds: float = 5,
    attempts: list[int] | None = None,
) -> MediaCrawlerAdapter:
    def before_request(attempt: int) -> bool:
        if attempts is not None:
            attempts.append(attempt)
        return True

    return MediaCrawlerAdapter(
        crawler_dir=crawler,
        output_dir=root / "output",
        owner_key="a" * 32,
        before_request=before_request,
        cancelled=lambda: False,
        max_requests=26,
        max_seconds=max_seconds,
        verify_revision=False,
    )


def _crawler_with_results(
    root: Path, contents: list[dict[str, object]], comments: list[dict[str, object]]
) -> Path:
    content_fixture = root / "contents.jsonl"
    comment_fixture = root / "comments.jsonl"
    content_fixture.write_text(
        "".join(json.dumps(row, ensure_ascii=False) + "\n" for row in contents), encoding="utf-8"
    )
    comment_fixture.write_text(
        "".join(json.dumps(row, ensure_ascii=False) + "\n" for row in comments), encoding="utf-8"
    )
    script = "\n".join(
        (
            "import pathlib, shutil, sys",
            "root = pathlib.Path(sys.argv[sys.argv.index('--save_data_path') + 1])",
            "root = root / 'bili' / 'jsonl'",
            "root.mkdir(parents=True)",
            f"shutil.copyfile({str(content_fixture)!r}, root / 'search_contents_fixture.jsonl')",
            f"shutil.copyfile({str(comment_fixture)!r}, root / 'search_comments_fixture.jsonl')",
        )
    )
    return _fake_crawler(root, script)


def _video(video_id: str, published_at: datetime | None) -> dict[str, object]:
    return {
        "video_id": video_id,
        "video_url": f"https://www.bilibili.com/video/av{video_id}",
        "title": f"Video {video_id}",
        "create_time": int(published_at.timestamp()) if published_at is not None else None,
    }


def _video_comment(video_id: str) -> dict[str, object]:
    return {
        "video_id": video_id,
        "comment_id": video_id,
        "parent_comment_id": "0",
        "content": f"Comment {video_id}",
    }


def test_empty_video_description_does_not_emit_an_empty_body() -> None:
    post = _post(
        {
            "video_id": "117335721579544",
            "video_url": "https://www.bilibili.com/video/av117335721579544",
            "title": "DeepSeek 折叠屏",
            "desc": "",
            "create_time": 1790401063,
        }
    )

    assert post.text is None
    assert "body" not in KeywordDiscoveryPageCommitService._payload(post)


def test_search_maps_real_shape_and_comments_read_only_cached_output(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    content = {
        "video_id": "116475771755099",
        "title": "DeepSeek 新闻",
        "desc": "公开视频简介",
        "create_time": 1781186796,
        "nickname": "创作者",
        "creator_hash": "creator-1",
        "liked_count": "12",
        "video_play_count": "345",
        "video_comment": "1",
        "video_share_count": "2",
        "video_url": "https://www.bilibili.com/video/av116475771755099",
    }
    comment = {
        "comment_id": "302351534321",
        "parent_comment_id": "0",
        "create_time": 1781186796,
        "video_id": content["video_id"],
        "content": "一级评论",
        "creator_hash": "commenter-1",
        "nickname": "评论者",
        "like_count": 6,
    }
    content_fixture = tmp_path / "contents.jsonl"
    comment_fixture = tmp_path / "comments.jsonl"
    content_fixture.write_text(json.dumps(content, ensure_ascii=False) + "\n", encoding="utf-8")
    comment_fixture.write_text(json.dumps(comment, ensure_ascii=False) + "\n", encoding="utf-8")
    script = "\n".join(
        (
            "import os, pathlib, shutil, sys",
            "assert '--lt' in sys.argv and sys.argv[sys.argv.index('--lt') + 1] == 'qrcode'",
            "assert sys.argv[sys.argv.index('--platform') + 1] == 'bili'",
            "assert 'HOTKEY_TEST_SECRET' not in os.environ",
            "root = pathlib.Path(sys.argv[sys.argv.index('--save_data_path') + 1])",
            "root = root / 'bili' / 'jsonl'",
            "root.mkdir(parents=True)",
            f"shutil.copyfile({str(content_fixture)!r}, root / 'search_contents_2026-09-26.jsonl')",
            f"shutil.copyfile({str(comment_fixture)!r}, root / 'search_comments_2026-09-26.jsonl')",
        )
    )
    crawler = _fake_crawler(tmp_path, script)
    monkeypatch.setenv("HOTKEY_TEST_SECRET", "must-not-enter-child")
    attempts: list[int] = []
    adapter = _adapter(tmp_path, crawler, attempts=attempts)

    search = adapter.fetch_page(SearchRequest(source_key="bilibili", query="DeepSeek", page_size=2))

    assert search.state is SourcePageState.PARTIAL
    assert search.stop_reason is SourceStopReason.BUDGET_EXHAUSTED
    assert search.request_count == 14
    assert attempts == list(range(1, 15))
    assert len(search.items) == 1
    post = search.items[0]
    assert isinstance(post, SourcePost)
    assert post.external_id == content["video_id"]
    assert (post.like_count, post.play_count, post.comment_count) == (12, 345, 1)

    comments = adapter.fetch_page(
        CommentsRequest(source_key="bilibili", post_external_id="116475771755099", page_size=20)
    )
    assert comments.request_count == 0
    assert comments.state is SourcePageState.PARTIAL
    assert len(comments.items) == 1
    assert isinstance(comments.items[0], SourceComment)
    assert comments.items[0].parent_comment_external_id is None
    assert attempts == list(range(1, 15))


def test_search_filters_outside_window_and_its_comments(
    tmp_path: Path, caplog: pytest.LogCaptureFixture
) -> None:
    caplog.set_level(logging.INFO, logger="sources.adapters.mediacrawler")
    starts_at = datetime(2026, 9, 24, 23, 29, tzinfo=UTC)
    ends_at = datetime(2026, 9, 26, 5, 29, tzinfo=UTC)
    times = {
        "101": datetime(2026, 9, 24, 23, 28, 59, tzinfo=UTC),
        "102": starts_at,
        "103": ends_at,
        "104": datetime(2026, 9, 26, 5, 29, 1, tzinfo=UTC),
        "105": None,
    }
    crawler = _crawler_with_results(
        tmp_path,
        [_video(video_id, published_at) for video_id, published_at in times.items()],
        [_video_comment(video_id) for video_id in times],
    )
    adapter = _adapter(tmp_path, crawler)

    page = adapter.fetch_page(
        SearchRequest(
            source_key="bilibili",
            query="DeepSeek",
            page_size=5,
            starts_at=starts_at,
            ends_at=ends_at,
        )
    )

    assert page.state is SourcePageState.PARTIAL
    assert [post.external_id for post in page.items] == ["102", "103", "105"]
    assert "filtered 2 posts and 2 comments" in caplog.text
    for video_id in times:
        index = tmp_path / "output" / ("a" * 32) / "index" / f"{video_id}.json"
        assert index.exists() is (video_id in {"102", "103", "105"})
        comments = adapter.fetch_page(
            CommentsRequest(source_key="bilibili", post_external_id=video_id, page_size=20)
        )
        if video_id in {"102", "103", "105"}:
            assert [comment.post_external_id for comment in comments.items] == [video_id]
        else:
            assert comments.state is SourcePageState.STOPPED
            assert comments.stop_reason is SourceStopReason.NOT_FOUND


def test_search_returns_empty_when_all_posts_are_outside_window(tmp_path: Path) -> None:
    crawler = _crawler_with_results(
        tmp_path,
        [_video("101", datetime(2025, 8, 1, tzinfo=UTC))],
        [_video_comment("101")],
    )

    page = _adapter(tmp_path, crawler).fetch_page(
        SearchRequest(
            source_key="bilibili",
            query="DeepSeek",
            page_size=1,
            starts_at=datetime(2026, 9, 24, 23, 29, tzinfo=UTC),
            ends_at=datetime(2026, 9, 26, 5, 29, tzinfo=UTC),
        )
    )

    assert page.state is SourcePageState.EMPTY
    assert page.items == ()
    assert page.stop_reason is SourceStopReason.SOURCE_EMPTY
    assert not (tmp_path / "output" / ("a" * 32) / "index" / "101.json").exists()


@pytest.mark.parametrize(
    ("revision", "accepted"),
    [
        ("fb4e6c57ade1c7a2b3a61e69abc4fd4130047eb2", True),
        ("53bfdf9116b737575488eb05df2bbcaf4dbf05b9", False),
    ],
)
def test_reviewed_revision_is_required(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch, revision: str, accepted: bool
) -> None:
    crawler = _fake_crawler(tmp_path, "")
    adapter = MediaCrawlerAdapter(
        crawler_dir=crawler,
        output_dir=tmp_path / "output",
        owner_key="a" * 32,
        before_request=lambda _: True,
        cancelled=lambda: False,
        max_requests=26,
        max_seconds=5,
    )

    def fake_run(command: list[str], **_: object) -> subprocess.CompletedProcess[str]:
        output = f"{revision}\n" if command[1] == "rev-parse" else ""
        return subprocess.CompletedProcess(command, 0, output, "")

    monkeypatch.setattr("sources.adapters.mediacrawler.subprocess.run", fake_run)
    if accepted:
        adapter._validate_crawler()
    else:
        with pytest.raises(ValueError, match="reviewed hotkey-safe build"):
            adapter._validate_crawler()


@pytest.mark.parametrize(
    ("signal_text", "expected"),
    [
        ("请完成验证码", SourceStopReason.AUTHENTICATION_REQUIRED),
        ("访问频繁", SourceStopReason.RATE_LIMITED),
    ],
)
def test_login_and_rate_signals_stop_without_retry(
    tmp_path: Path, signal_text: str, expected: SourceStopReason
) -> None:
    crawler = _fake_crawler(
        tmp_path,
        f"print({signal_text!r})\nraise SystemExit(1)\n",
    )
    page = _adapter(tmp_path, crawler).fetch_page(
        SearchRequest(source_key="bilibili", query="DeepSeek", page_size=2)
    )

    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is expected
    assert page.request_count == 14


def test_timeout_stops_the_process_group(tmp_path: Path) -> None:
    crawler = _fake_crawler(tmp_path, "import time\ntime.sleep(30)\n")

    page = _adapter(tmp_path, crawler, max_seconds=0.1).fetch_page(
        SearchRequest(source_key="bilibili", query="DeepSeek", page_size=2)
    )

    assert page.state is SourcePageState.STOPPED
    assert page.stop_reason is SourceStopReason.BUDGET_EXHAUSTED


def test_expired_login_interrupts_child_before_qrcode_wait(tmp_path: Path) -> None:
    crawler = _fake_crawler(
        tmp_path,
        "import time\nprint('[BilibiliLogin.login_by_qrcode] Begin login', flush=True)\n"
        "time.sleep(30)\n",
    )

    page = _adapter(tmp_path, crawler).fetch_page(
        SearchRequest(source_key="bilibili", query="DeepSeek", page_size=2)
    )

    assert page.stop_reason is SourceStopReason.AUTHENTICATION_REQUIRED


def test_keyword_list_is_rejected_before_launch(tmp_path: Path) -> None:
    crawler = _fake_crawler(tmp_path, "raise RuntimeError('should not run')\n")
    attempts: list[int] = []

    page = _adapter(tmp_path, crawler, attempts=attempts).fetch_page(
        SearchRequest(source_key="bilibili", query="DeepSeek,Claude", page_size=2)
    )

    assert page.stop_reason is SourceStopReason.UNSUPPORTED
    assert attempts == []
