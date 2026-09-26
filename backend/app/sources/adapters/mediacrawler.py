"""Bounded local MediaCrawler bridge for Bilibili search and cached comments."""

from __future__ import annotations

import fcntl
import json
import logging
import os
import re
import signal
import subprocess
import time
from collections.abc import Callable
from contextlib import suppress
from datetime import UTC, datetime
from pathlib import Path
from uuid import uuid4

from sources.contracts import (
    AuthorPostsRequest,
    CommentsRequest,
    RepliesRequest,
    SearchRequest,
    SocialSourceCapability,
    SourceCapability,
    SourceComment,
    SourcePage,
    SourcePageState,
    SourcePost,
    SourceStopReason,
)

SOURCE_KEY = "bilibili"
ADAPTER_VERSION = "mediacrawler-fb4e6c5-hotkey-safe"
_PINNED_REVISION = "fb4e6c57ade1c7a2b3a61e69abc4fd4130047eb2"
_LOGGER = logging.getLogger(__name__)
_MAX_OUTPUT_BYTES = 2 * 1024 * 1024
_MAX_LOG_BYTES = 128 * 1024
_AUTH_SIGNAL = re.compile(
    r"验证码|滑块|请扫码|扫码登录|登录失效|未登录|captcha|login required|"
    r"login_by_qrcode.*Begin|Waiting for scan code login",
    re.I,
)
_RATE_SIGNAL = re.compile(r"访问频繁|请求频繁|风控|rate.limit|too many requests|blocked", re.I)
_VIDEO_ID = re.compile(r"^[0-9]{1,30}$")


def _counter(value: object) -> int | None:
    if value in (None, ""):
        return None
    if isinstance(value, bool) or not str(value).isdigit():
        raise ValueError("invalid MediaCrawler counter")
    return int(str(value))


def _timestamp(value: object) -> datetime | None:
    count = _counter(value)
    return datetime.fromtimestamp(count, UTC) if count else None


def _read_rows(path: Path, *, limit: int) -> tuple[dict[str, object], ...]:
    if path.is_symlink() or not path.is_file() or path.stat().st_size > _MAX_OUTPUT_BYTES:
        raise ValueError("unsafe MediaCrawler result file")
    rows: list[dict[str, object]] = []
    with path.open(encoding="utf-8") as stream:
        for line in stream:
            if len(rows) >= limit:
                raise ValueError("MediaCrawler result exceeds the requested cap")
            value = json.loads(line)
            if not isinstance(value, dict):
                raise ValueError("MediaCrawler result row must be an object")
            rows.append(value)
    return tuple(rows)


def _post(row: dict[str, object]) -> SourcePost:
    video_id = str(row["video_id"])
    if _VIDEO_ID.fullmatch(video_id) is None:
        raise ValueError("invalid Bilibili video ID")
    url = str(row["video_url"])
    if url != f"https://www.bilibili.com/video/av{video_id}":
        raise ValueError("invalid Bilibili video URL")
    return SourcePost(
        source_key=SOURCE_KEY,
        external_id=video_id,
        author_external_id=str(row["creator_hash"]) if row.get("creator_hash") else None,
        author_name=str(row["nickname"]) if row.get("nickname") else None,
        published_at=_timestamp(row.get("create_time")),
        title=str(row["title"]),
        text=str(row["desc"]) if row.get("desc") else None,
        text_scope="full",
        like_count=_counter(row.get("liked_count")),
        comment_count=_counter(row.get("video_comment")),
        repost_count=_counter(row.get("video_share_count")),
        play_count=_counter(row.get("video_play_count")),
        danmaku_count=_counter(row.get("video_danmaku")),
        canonical_url=url,
    )


def _comment(row: dict[str, object], video_id: str) -> SourceComment:
    if str(row.get("video_id")) != video_id:
        raise ValueError("comment belongs to a different video")
    parent = str(row.get("parent_comment_id") or "0")
    if _VIDEO_ID.fullmatch(str(row["comment_id"])) is None or (
        parent != "0" and _VIDEO_ID.fullmatch(parent) is None
    ):
        raise ValueError("invalid Bilibili comment ID")
    return SourceComment(
        source_key=SOURCE_KEY,
        external_id=str(row["comment_id"]),
        post_external_id=video_id,
        parent_comment_external_id=None if parent == "0" else parent,
        author_external_id=str(row["creator_hash"]) if row.get("creator_hash") else None,
        author_name=str(row["nickname"]) if row.get("nickname") else None,
        published_at=_timestamp(row.get("create_time")),
        text=str(row["content"]),
        like_count=_counter(row.get("like_count")),
    )


class MediaCrawlerAdapter:
    """One search invocation; comments only read the saved JSONL from that invocation."""

    source_key = SOURCE_KEY
    capabilities: frozenset[SocialSourceCapability] = frozenset(
        {SourceCapability.SEARCH, SourceCapability.COMMENTS}
    )

    def __init__(
        self,
        *,
        crawler_dir: Path,
        output_dir: Path,
        owner_key: str,
        before_request: Callable[[int], bool],
        cancelled: Callable[[], bool],
        max_requests: int,
        max_seconds: float,
        verify_revision: bool = True,
    ) -> None:
        self._crawler_dir = crawler_dir.expanduser()
        self._output_dir = output_dir.expanduser()
        self._owner_key = owner_key
        self._before_request = before_request
        self._cancelled = cancelled
        self._max_requests = max_requests
        self._max_seconds = max_seconds
        self._verify_revision = verify_revision

    def fetch_page(
        self, request: SearchRequest | AuthorPostsRequest | CommentsRequest | RepliesRequest
    ) -> SourcePage:
        if request.source_key != SOURCE_KEY or request.page_token is not None:
            return self._stopped(request.capability, SourceStopReason.UNSUPPORTED)
        if isinstance(request, CommentsRequest):
            return self._cached_comments(request)
        if not isinstance(request, SearchRequest):
            return self._stopped(request.capability, SourceStopReason.UNSUPPORTED)
        if request.page_size > 5 or "," in request.query:
            return self._stopped(SourceCapability.SEARCH, SourceStopReason.UNSUPPORTED)
        if self._cancelled():
            return self._stopped(SourceCapability.SEARCH, SourceStopReason.CANCELLED)
        self._validate_crawler()
        # MediaCrawler performs login checks and can retry each comment request.
        # Charge a conservative upper bound before allowing the child to start.
        charged = 6 + 4 * request.page_size
        if charged > self._max_requests:
            return self._stopped(SourceCapability.SEARCH, SourceStopReason.BUDGET_EXHAUSTED)
        for attempt in range(1, charged + 1):
            if not self._before_request(attempt):
                return self._stopped(
                    SourceCapability.SEARCH,
                    SourceStopReason.BUDGET_EXHAUSTED,
                    request_count=attempt - 1,
                )
        return self._search(request, charged)

    def _search(self, request: SearchRequest, charged: int) -> SourcePage:
        self._output_dir.mkdir(mode=0o700, parents=True, exist_ok=True)
        if self._output_dir.is_symlink() or self._output_dir.stat().st_mode & 0o077:
            raise ValueError("MediaCrawler output base is not private")
        root = self._output_dir / self._owner_key
        root.mkdir(mode=0o700, parents=True, exist_ok=True)
        if root.is_symlink() or root.stat().st_mode & 0o077:
            raise ValueError("MediaCrawler output directory is not private")
        run_dir = root / "runs" / uuid4().hex
        run_dir.mkdir(mode=0o700, parents=True)
        lock_path = self._output_dir / ".process.lock"
        lock_path.touch(mode=0o600, exist_ok=True)
        with lock_path.open("r+b") as lock:
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                return self._stopped(
                    SourceCapability.SEARCH,
                    SourceStopReason.BUDGET_EXHAUSTED,
                    request_count=charged,
                )
            result = self._run_child(request, run_dir)
        if result is not None:
            return self._stopped(SourceCapability.SEARCH, result, request_count=charged)
        files = tuple((run_dir / "bili" / "jsonl").glob("search_contents_*.jsonl"))
        if len(files) != 1:
            raise ValueError("MediaCrawler search result is missing or ambiguous")
        rows = _read_rows(files[0], limit=request.page_size)
        all_posts = tuple(_post(row) for row in rows)
        if request.starts_at is not None and request.ends_at is not None:
            posts = tuple(
                post
                for post in all_posts
                if post.published_at is None
                or request.starts_at <= post.published_at <= request.ends_at
            )
        else:
            posts = all_posts
        comment_files = tuple((run_dir / "bili" / "jsonl").glob("search_comments_*.jsonl"))
        if len(comment_files) > 1:
            raise ValueError("MediaCrawler comments result is ambiguous")
        filtered_comments = 0
        if comment_files:
            comments = _read_rows(comment_files[0], limit=request.page_size * 20)
            all_ids = {post.external_id for post in all_posts}
            retained_ids = {post.external_id for post in posts}
            counts: dict[str, int] = {}
            for comment in comments:
                video_id = str(comment.get("video_id"))
                if video_id not in all_ids:
                    raise ValueError("MediaCrawler returned unrelated comments")
                if video_id not in retained_ids:
                    filtered_comments += 1
                    continue
                counts[video_id] = counts.get(video_id, 0) + 1
                if counts[video_id] > 20:
                    raise ValueError("MediaCrawler exceeded the comment cap")
        filtered_posts = len(all_posts) - len(posts)
        if filtered_posts:
            _LOGGER.info(
                "MediaCrawler filtered %d posts and %d comments outside the requested window",
                filtered_posts,
                filtered_comments,
            )
        for post in posts:
            index = root / "index" / f"{post.external_id}.json"
            index.parent.mkdir(mode=0o700, exist_ok=True)
            temporary = index.with_name(f".{post.external_id}.{uuid4().hex}.tmp")
            temporary.write_text(json.dumps({"run": run_dir.name}), encoding="utf-8")
            temporary.chmod(0o600)
            temporary.replace(index)
        return SourcePage(
            source_key=SOURCE_KEY,
            capability=SourceCapability.SEARCH,
            state=SourcePageState.PARTIAL if posts else SourcePageState.EMPTY,
            items=posts,
            next_page_token=None,
            watermark=None,
            stop_reason=(
                SourceStopReason.BUDGET_EXHAUSTED if posts else SourceStopReason.SOURCE_EMPTY
            ),
            observed_at=datetime.now(UTC),
            request_count=charged,
            adapter_version=ADAPTER_VERSION,
        )

    def _run_child(self, request: SearchRequest, run_dir: Path) -> SourceStopReason | None:
        executable = self._crawler_dir / ".venv" / "bin" / "python"
        command = [
            str(executable),
            "main.py",
            "--platform",
            "bili",
            "--lt",
            "qrcode",
            "--type",
            "search",
            "--keywords",
            request.query,
            "--crawler_max_notes_count",
            str(request.page_size),
            "--save_data_option",
            "jsonl",
            "--save_data_path",
            str(run_dir),
        ]
        allowed = ("PATH", "HOME", "LANG", "DISPLAY", "XDG_RUNTIME_DIR", "TMPDIR")
        child_env = {key: os.environ[key] for key in allowed if key in os.environ}
        log_path = run_dir / "run.log"
        with log_path.open("wb") as output:
            process = subprocess.Popen(
                command,
                cwd=self._crawler_dir,
                env=child_env,
                stdout=output,
                stderr=subprocess.STDOUT,
                start_new_session=True,
            )
            deadline = time.monotonic() + self._max_seconds
            try:
                while process.poll() is None:
                    output.flush()
                    current_log = self._log_tail(log_path)
                    if _AUTH_SIGNAL.search(current_log):
                        self._terminate_process_group(process)
                        return SourceStopReason.AUTHENTICATION_REQUIRED
                    if _RATE_SIGNAL.search(current_log):
                        self._terminate_process_group(process)
                        return SourceStopReason.RATE_LIMITED
                    cancelled = self._cancelled()
                    if cancelled or time.monotonic() >= deadline:
                        self._terminate_process_group(process)
                        return (
                            SourceStopReason.CANCELLED
                            if cancelled
                            else SourceStopReason.BUDGET_EXHAUSTED
                        )
                    time.sleep(0.2)
            finally:
                if process.poll() is None:
                    self._terminate_process_group(process)
        log_size = log_path.stat().st_size
        output_text = self._log_tail(log_path)
        if _AUTH_SIGNAL.search(output_text):
            return SourceStopReason.AUTHENTICATION_REQUIRED
        if _RATE_SIGNAL.search(output_text):
            return SourceStopReason.RATE_LIMITED
        if process.returncode != 0:
            return SourceStopReason.AUTHENTICATION_REQUIRED
        if log_size > _MAX_LOG_BYTES:
            return SourceStopReason.PROTOCOL_ERROR
        return None

    @staticmethod
    def _log_tail(path: Path) -> str:
        with path.open("rb") as log:
            log.seek(max(0, path.stat().st_size - _MAX_LOG_BYTES))
            return log.read(_MAX_LOG_BYTES).decode("utf-8", errors="replace")

    @staticmethod
    def _terminate_process_group(process: subprocess.Popen[bytes]) -> None:
        with suppress(ProcessLookupError):
            os.killpg(process.pid, signal.SIGTERM)
        with suppress(subprocess.TimeoutExpired):
            process.wait(timeout=2)
        with suppress(ProcessLookupError):
            os.killpg(process.pid, signal.SIGKILL)
        process.wait(timeout=2)

    def _validate_crawler(self) -> None:
        if not (
            (self._crawler_dir / ".venv" / "bin" / "python").is_file()
            and (self._crawler_dir / "main.py").is_file()
        ):
            raise ValueError("MediaCrawler installation is unavailable")
        if not self._verify_revision:
            return
        child_env = {key: os.environ[key] for key in ("PATH", "HOME", "LANG") if key in os.environ}
        revision = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=self._crawler_dir,
            env=child_env,
            capture_output=True,
            text=True,
            timeout=3,
            check=False,
        )
        tracked_changes = subprocess.run(
            ["git", "status", "--porcelain", "--untracked-files=no"],
            cwd=self._crawler_dir,
            env=child_env,
            capture_output=True,
            text=True,
            timeout=3,
            check=False,
        )
        if (
            revision.returncode != 0
            or revision.stdout.strip() != _PINNED_REVISION
            or tracked_changes.returncode != 0
            or tracked_changes.stdout.strip()
        ):
            raise ValueError("MediaCrawler revision differs from the reviewed hotkey-safe build")

    def _cached_comments(self, request: CommentsRequest) -> SourcePage:
        video_id = request.post_external_id
        if _VIDEO_ID.fullmatch(video_id) is None or request.page_size > 20:
            return self._stopped(SourceCapability.COMMENTS, SourceStopReason.UNSUPPORTED)
        root = self._output_dir / self._owner_key
        index = root / "index" / f"{video_id}.json"
        if index.is_symlink() or not index.is_file() or index.stat().st_size > 128:
            return self._stopped(SourceCapability.COMMENTS, SourceStopReason.NOT_FOUND)
        record = json.loads(index.read_text(encoding="utf-8"))
        run_name = record.get("run") if isinstance(record, dict) else None
        if not isinstance(run_name, str) or re.fullmatch(r"[0-9a-f]{32}", run_name) is None:
            raise ValueError("invalid MediaCrawler run reference")
        run_dir = root / "runs" / run_name
        files = tuple((run_dir / "bili" / "jsonl").glob("search_comments_*.jsonl"))
        if len(files) != 1:
            return self._stopped(SourceCapability.COMMENTS, SourceStopReason.NOT_FOUND)
        rows = _read_rows(files[0], limit=100)
        comments = tuple(
            _comment(row, video_id) for row in rows if str(row.get("video_id")) == video_id
        )
        if len(comments) > 20:
            raise ValueError("MediaCrawler exceeded the comment cap")
        comments = comments[: request.page_size]
        return SourcePage(
            source_key=SOURCE_KEY,
            capability=SourceCapability.COMMENTS,
            state=SourcePageState.PARTIAL if comments else SourcePageState.EMPTY,
            items=comments,
            next_page_token=None,
            watermark=None,
            stop_reason=(
                SourceStopReason.BUDGET_EXHAUSTED if comments else SourceStopReason.SOURCE_EMPTY
            ),
            observed_at=datetime.now(UTC),
            request_count=0,
            adapter_version=ADAPTER_VERSION,
        )

    @staticmethod
    def _stopped(
        capability: SocialSourceCapability,
        reason: SourceStopReason,
        *,
        request_count: int = 0,
    ) -> SourcePage:
        return SourcePage(
            source_key=SOURCE_KEY,
            capability=capability,
            state=SourcePageState.STOPPED,
            items=(),
            next_page_token=None,
            watermark=None,
            stop_reason=reason,
            observed_at=datetime.now(UTC),
            request_count=request_count,
            adapter_version=ADAPTER_VERSION,
        )
