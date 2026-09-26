from __future__ import annotations

import hashlib
import json
import os
import stat
import tempfile
import unicodedata
from pathlib import Path
from uuid import UUID
from zoneinfo import ZoneInfo

from knowledge.schemas import DailyExportInput, KnowledgeExportResult

BEGIN = "<!-- hotkey:begin -->"
END = "<!-- hotkey:end -->"
LOCAL_TIMEZONE = ZoneInfo("Asia/Shanghai")
MAX_TITLE_LENGTH = 80


def safe_title(value: str) -> str:
    cleaned = "".join(
        character
        for character in value
        if character not in '/\\:*?"<>|' and not unicodedata.category(character).startswith("C")
    )
    cleaned = " ".join(cleaned.split()).strip(" .")
    return cleaned[:MAX_TITLE_LENGTH].rstrip(" .") or "未命名"


def daily_relative_path(note: DailyExportInput, *, short_id: bool = False) -> Path:
    date = note.window_start.astimezone(LOCAL_TIMEZONE).date().isoformat()
    title = safe_title(note.topic_name)
    suffix = f"-{str(note.topic_id)[:8]}" if short_id else ""
    return Path("日报") / f"{date} {title}{suffix}.md"


def _managed_text(note: DailyExportInput, object_id: UUID) -> str:
    date = note.window_start.astimezone(LOCAL_TIMEZONE).date().isoformat()
    frontmatter = {
        "hotkey_id": str(object_id),
        "type": "daily",
        "topic": note.topic_name,
        "date": date,
        "generated_at": note.generated_at.isoformat(),
        "generator": note.generator,
        "source_url": note.source_url,
    }
    properties = "\n".join(
        f"{key}: {json.dumps(value, ensure_ascii=False)}" for key, value in frontmatter.items()
    )
    return f"---\n{properties}\n---\n\n{BEGIN}\n{note.body_markdown.rstrip()}\n{END}\n"


def content_sha256(note: DailyExportInput) -> str:
    """Hash only the managed block; user text and preserved frontmatter are outside it."""
    managed_block = f"{BEGIN}\n{note.body_markdown.rstrip()}\n{END}"
    return hashlib.sha256(managed_block.encode("utf-8")).hexdigest()


def _require_plain_directory(path: Path) -> None:
    if path.is_symlink() or not path.is_dir():
        raise OSError("Obsidian directory is absent or is a symbolic link")
    if not stat.S_IMODE(path.stat().st_mode) & 0o222 or not os.access(path, os.W_OK):
        raise PermissionError("Obsidian directory is not writable")


def _target(vault: Path, root: str, relative_path: Path) -> Path:
    if root in {"", ".", ".."} or Path(root).name != root:
        raise ValueError("invalid Obsidian root")
    if relative_path.is_absolute() or any(part in {"", ".", ".."} for part in relative_path.parts):
        raise ValueError("invalid Obsidian relative path")
    _require_plain_directory(vault)
    root_path = vault / root
    if root_path.exists() or root_path.is_symlink():
        _require_plain_directory(root_path)
    else:
        root_path.mkdir(mode=0o700)
    current = root_path
    for part in relative_path.parts[:-1]:
        current = current / part
        if current.exists() or current.is_symlink():
            _require_plain_directory(current)
        else:
            current.mkdir(mode=0o700)
    target = current / relative_path.name
    if target.is_symlink():
        raise OSError("Obsidian target is a symbolic link")
    if not target.resolve(strict=False).is_relative_to(root_path.resolve(strict=True)):
        raise ValueError("Obsidian path escapes HotKey root")
    return target


def _merge(existing: str, generated: str) -> str:
    if existing.count(BEGIN) != 1 or existing.count(END) != 1:
        raise ValueError("existing Obsidian note has no unique managed block")
    before, rest = existing.split(BEGIN, 1)
    _, after = rest.split(END, 1)
    new_block = generated.split(BEGIN, 1)[1].split(END, 1)[0]
    return f"{before}{BEGIN}{new_block}{END}{after}"


def write_daily_note(
    vault: Path,
    root: str,
    note: DailyExportInput,
    object_id: UUID,
    *,
    relative_path: Path | None = None,
) -> KnowledgeExportResult:
    path = relative_path or daily_relative_path(note)
    target = _target(vault, root, path)
    if relative_path is None and target.exists():
        existing_header = target.read_text(encoding="utf-8").split(BEGIN, 1)[0]
        if f'hotkey_id: "{object_id}"' not in existing_header:
            path = daily_relative_path(note, short_id=True)
            target = _target(vault, root, path)
    generated = _managed_text(note, object_id)
    if target.exists():
        existing = target.read_text(encoding="utf-8")
        if f'hotkey_id: "{object_id}"' not in existing.split(BEGIN, 1)[0]:
            raise ValueError("Obsidian note belongs to another object")
        updated = _merge(existing, generated)
    else:
        updated = f"{generated}\n## 我的笔记\n"
    digest = content_sha256(note)
    if target.exists() and updated == existing:
        return KnowledgeExportResult(
            relative_path=(Path(root) / path).as_posix(),
            content_sha256=digest,
            written=False,
        )
    temp_name: str | None = None
    try:
        with tempfile.NamedTemporaryFile(
            mode="w", encoding="utf-8", dir=target.parent, prefix=".hotkey-", delete=False
        ) as temporary:
            temp_name = temporary.name
            temporary.write(updated)
            temporary.flush()
            os.fsync(temporary.fileno())
        os.replace(temp_name, target)
        temp_name = None
        directory_fd = os.open(target.parent, os.O_RDONLY)
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
    finally:
        if temp_name is not None:
            Path(temp_name).unlink(missing_ok=True)
    return KnowledgeExportResult(
        relative_path=(Path(root) / path).as_posix(),
        content_sha256=digest,
        written=True,
    )
