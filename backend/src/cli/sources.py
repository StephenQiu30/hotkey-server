"""Explicit source probes run without database or broker credentials."""

import argparse
import json

import httpx
from pydantic import ValidationError

from sources.adapters.bilibili import Bilibili
from sources.adapters.bluesky import Bluesky
from sources.schemas import (
    BilibiliCommentsInput,
    BilibiliPostInput,
    BilibiliRepliesInput,
    BilibiliSearchInput,
    SearchInput,
    ThreadInput,
)


def register(parser: argparse.ArgumentParser) -> None:
    platforms = parser.add_subparsers(dest="source_platform", required=True)
    bluesky = platforms.add_parser("bluesky")
    bluesky_operations = bluesky.add_subparsers(dest="source_operation", required=True)
    search = bluesky_operations.add_parser("search")
    search.add_argument("--keyword", required=True)
    search.add_argument("--since", required=True)
    search.add_argument("--until", required=True)
    search.add_argument("--limit", type=int, default=20)
    search.add_argument("--cursor")
    thread = bluesky_operations.add_parser("thread")
    thread.add_argument("--uri", required=True)
    thread.add_argument("--depth", type=int, default=2)
    thread.add_argument("--max-nodes", type=int, default=100)

    bilibili = platforms.add_parser("bilibili")
    bilibili_operations = bilibili.add_subparsers(dest="source_operation", required=True)
    bilibili_search = bilibili_operations.add_parser("search")
    bilibili_search.add_argument("--keyword", required=True)
    bilibili_search.add_argument("--page", type=int, default=1)
    bilibili_search.add_argument("--limit", type=int, default=20)
    post = bilibili_operations.add_parser("post")
    post.add_argument("--bvid", required=True)
    comments = bilibili_operations.add_parser("comments")
    comments.add_argument("--aid", type=int, required=True)
    comments.add_argument("--cursor", type=int, default=0)
    comments.add_argument("--limit", type=int, default=20)
    replies = bilibili_operations.add_parser("replies")
    replies.add_argument("--aid", type=int, required=True)
    replies.add_argument("--root-id", type=int, required=True)
    replies.add_argument("--page", type=int, default=1)
    replies.add_argument("--limit", type=int, default=20)


def run(args: argparse.Namespace) -> None:
    try:
        # Independent public client: no ambient cookies, credentials or proxy injection.
        with httpx.Client(trust_env=False) as client:
            if args.source_platform == "bluesky":
                source = Bluesky(client)
                if args.source_operation == "search":
                    result = source.search(
                        SearchInput.model_validate(
                            {
                                "keyword": args.keyword,
                                "since": args.since,
                                "until": args.until,
                                "limit": args.limit,
                                "cursor": args.cursor,
                            }
                        )
                    )
                else:
                    result = source.thread(
                        ThreadInput(uri=args.uri, depth=args.depth, max_nodes=args.max_nodes)
                    )
            else:
                bilibili = Bilibili(client)
                if args.source_operation == "search":
                    result = bilibili.search(
                        BilibiliSearchInput(keyword=args.keyword, page=args.page, limit=args.limit)
                    )
                elif args.source_operation == "post":
                    result = bilibili.post(BilibiliPostInput(bvid=args.bvid))
                elif args.source_operation == "comments":
                    result = bilibili.comments(
                        BilibiliCommentsInput(aid=args.aid, cursor=args.cursor, limit=args.limit)
                    )
                else:
                    result = bilibili.replies(
                        BilibiliRepliesInput(
                            aid=args.aid,
                            root_id=args.root_id,
                            page=args.page,
                            limit=args.limit,
                        )
                    )
    except ValidationError:
        print(json.dumps({"status": "failed", "code": "validation_failed"}))
        raise SystemExit(2) from None
    print(result.model_dump_json())
    if result.status == "failed":
        raise SystemExit(1)
