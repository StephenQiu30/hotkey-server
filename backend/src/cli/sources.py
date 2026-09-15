"""Explicit source probes run without database or broker credentials."""

import argparse
import json

import httpx
from pydantic import ValidationError

from sources.adapters.bluesky import Bluesky
from sources.schemas import SearchInput, ThreadInput


def register(parser: argparse.ArgumentParser) -> None:
    operations = parser.add_subparsers(dest="source_operation", required=True)
    search = operations.add_parser("search")
    search.add_argument("--keyword", required=True)
    search.add_argument("--since", required=True)
    search.add_argument("--until", required=True)
    search.add_argument("--limit", type=int, default=20)
    search.add_argument("--cursor")
    thread = operations.add_parser("thread")
    thread.add_argument("--uri", required=True)
    thread.add_argument("--depth", type=int, default=2)
    thread.add_argument("--max-nodes", type=int, default=100)


def run(args: argparse.Namespace) -> None:
    try:
        # Independent public client: no ambient cookies, credentials or proxy injection.
        with httpx.Client(trust_env=False) as client:
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
    except ValidationError:
        print(json.dumps({"status": "failed", "code": "validation_failed"}))
        raise SystemExit(2) from None
    print(result.model_dump_json())
    if result.status == "failed":
        raise SystemExit(1)
