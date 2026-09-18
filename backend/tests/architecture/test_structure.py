from __future__ import annotations

import ast
from pathlib import Path

SRC = Path(__file__).resolve().parents[2] / "src"


def _imports(path: Path) -> set[str]:
    tree = ast.parse(path.read_text())
    names: set[str] = set()
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            names.update(alias.name for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module:
            names.add(node.module)
    return names


def test_backend_has_no_wrapper_package() -> None:
    assert not (SRC / "app").exists()
    assert not (SRC / "hotkey").exists()


def test_routers_do_not_import_persistence_or_service_implementations() -> None:
    for path in (SRC / "api" / "routers").glob("*.py"):
        imports = _imports(path)
        assert not any(name.startswith("sqlalchemy") for name in imports)
        assert not any(name.endswith(".models") for name in imports)
        assert not any(name.endswith(".services") for name in imports)


def test_schema_modules_do_not_depend_on_http_or_orm() -> None:
    for path in SRC.rglob("schemas.py"):
        imports = _imports(path)
        assert not any(name.startswith(("fastapi", "starlette", "sqlalchemy")) for name in imports)
