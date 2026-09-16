from __future__ import annotations

import argparse
import json
import os
from hashlib import sha256
from pathlib import Path

from pydantic import ValidationError
from sqlalchemy.orm import Session, sessionmaker

from contents.schemas import ContentWithdrawalManifest
from core.config import Settings
from evidence.adapters.minio import MinioEvidenceStore
from evidence.services import EvidenceDeletionService
from knowledge.services import KnowledgeService

MAX_MANIFEST_BYTES = 10 * 1024 * 1024


def register(subparsers: argparse._SubParsersAction[argparse.ArgumentParser]) -> None:
    export = subparsers.add_parser("deletion-export")
    export.add_argument("path", type=Path)
    replay = subparsers.add_parser("deletion-replay")
    replay.add_argument("path", type=Path)
    reconcile = subparsers.add_parser("evidence-reconcile")
    reconcile.add_argument("--limit", type=int, default=20, choices=range(1, 101))


def _write_private(path: Path, payload: bytes) -> None:
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    try:
        with os.fdopen(descriptor, "wb") as destination:
            destination.write(payload)
    except Exception:
        path.unlink(missing_ok=True)
        raise


def _read_manifest(path: Path) -> ContentWithdrawalManifest:
    size = path.stat().st_size
    if size > MAX_MANIFEST_BYTES:
        raise RuntimeError("withdrawal_manifest_too_large")
    try:
        return ContentWithdrawalManifest.model_validate_json(path.read_bytes())
    except ValidationError as error:
        raise RuntimeError("withdrawal_manifest_invalid") from error


def run(
    args: argparse.Namespace,
    settings: Settings,
    factory: sessionmaker[Session],
) -> None:
    knowledge = KnowledgeService(factory)
    if args.command == "deletion-export":
        manifest = knowledge.export_withdrawal_manifest()
        payload = manifest.model_dump_json(indent=2).encode()
        _write_private(args.path, payload)
        print(
            json.dumps(
                {
                    "entries": len(manifest.entries),
                    "manifest_sha256": sha256(payload).hexdigest(),
                },
                sort_keys=True,
            )
        )
        return
    if args.command == "deletion-replay":
        manifest = _read_manifest(args.path)
        replay_result = knowledge.replay_withdrawal_manifest(manifest)
        print(json.dumps(replay_result.model_dump(), sort_keys=True))
        return
    store = MinioEvidenceStore.from_settings(settings)
    cleanup_result = EvidenceDeletionService(factory, store).reconcile(args.limit)
    print(
        json.dumps(
            {"deleted": cleanup_result.deleted, "failed": cleanup_result.failed},
            sort_keys=True,
        )
    )
