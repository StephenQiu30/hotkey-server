"""Validate public entry points without contacting external services."""

from pathlib import Path
import re
from urllib.parse import unquote

import yaml


ROOT = Path(__file__).resolve().parents[2]
DOCUMENTS = (
    "README.md",
    "CONTRIBUTING.md",
    "SECURITY.md",
    "docs/plans/001-热点舆情监控平台总计划.md",
    ".github/PULL_REQUEST_TEMPLATE.md",
)
LINK = re.compile(r"\[[^]]+\]\(([^)]+)\)")


def check_document(relative_path: str) -> None:
    path = ROOT / relative_path
    content = path.read_text(encoding="utf-8")
    for number, line in enumerate(content.splitlines(), 1):
        if line != line.rstrip():
            raise ValueError(f"{relative_path}:{number}: trailing whitespace")
    for target in LINK.findall(content):
        if target.startswith(("http://", "https://", "mailto:", "#")):
            continue
        local_path = unquote(target.split("#", 1)[0].split("?", 1)[0])
        if local_path and not (path.parent / local_path).is_file():
            raise ValueError(f"{relative_path}: missing link target {target}")


def check_issue_templates() -> None:
    template_dir = ROOT / ".github/ISSUE_TEMPLATE"
    for path in sorted(template_dir.glob("*.yml")):
        data = yaml.safe_load(path.read_text(encoding="utf-8"))
        if not isinstance(data, dict):
            raise ValueError(f"{path.name}: expected a YAML mapping")
        if path.name == "config.yml":
            if not isinstance(data.get("contact_links"), list):
                raise ValueError("config.yml: missing contact links")
        elif not all(data.get(key) for key in ("name", "description", "body")):
            raise ValueError(f"{path.name}: incomplete issue form")


if __name__ == "__main__":
    for document in DOCUMENTS:
        check_document(document)
    check_issue_templates()
    print("Public documents and GitHub issue templates: OK")
