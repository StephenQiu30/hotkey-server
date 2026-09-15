"""Executable import and naming boundaries for the modular FastAPI application."""

import ast
import re
from pathlib import Path

import pytest

SOURCE = Path(__file__).resolve().parents[2] / "src"
PACKAGES = {
    "ai",
    "analysis",
    "api",
    "collection",
    "contents",
    "core",
    "db",
    "events",
    "evidence",
    "identity",
    "monitors",
    "notifications",
    "jobs",
    "knowledge",
    "audit",
    "migrations",
    "tools",
    "worker",
    "cli",
    "sources",
}


def application_files():
    yield from (path for path in SOURCE.rglob("*.py") if "__pycache__" not in path.parts)


def unregistered_top_level_modules(paths):
    actual = {
        path.relative_to(SOURCE).parts[0]
        for path in paths
        if len(path.relative_to(SOURCE).parts) > 1
    }
    return actual - PACKAGES


def test_only_registered_top_level_modules_exist():
    unregistered = unregistered_top_level_modules(application_files())
    assert not unregistered, f"Register module boundaries for: {sorted(unregistered)}"


def test_unknown_top_level_module_is_rejected():
    assert unregistered_top_level_modules([SOURCE / "rogue" / "module.py"]) == {"rogue"}


def imports(tree):
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            yield from (alias.name for alias in node.names)
        elif isinstance(node, ast.ImportFrom):
            yield node.module or ""
            yield from (f"{node.module}.{alias.name}" for alias in node.names)


def forbidden_imports(relative, tree):
    dependencies = list(imports(tree))
    if relative.startswith("ai/"):
        return [
            d
            for d in dependencies
            if d.split(".")[0]
            in {
                "api",
                "main",
                "db",
                "sqlalchemy",
                "worker",
                "cli",
                "celery",
                "kombu",
                "analysis",
                "knowledge",
            }
        ]
    if relative.startswith("evidence/adapters/"):
        return [
            d
            for d in dependencies
            if d.split(".")[0]
            in {"api", "main", "db", "sqlalchemy", "worker", "cli", "celery", "kombu"}
        ]
    if relative.startswith("sources/adapters/"):
        return [
            d
            for d in dependencies
            if d.split(".")[0]
            in {"api", "main", "db", "sqlalchemy", "worker", "cli", "celery", "kombu"}
        ]
    if relative.startswith("sources/") and relative not in (
        "sources/schemas.py",
        "sources/services.py",
    ):
        return [
            d
            for d in dependencies
            if d.split(".")[0]
            in {"api", "main", "db", "sqlalchemy", "worker", "cli", "celery", "kombu"}
        ]
    if relative.startswith("api/routers/"):
        return [
            d
            for d in dependencies
            if d.startswith(("sqlalchemy", "celery", "kombu", "db", "worker", "cli"))
            or d.endswith((".models", ".messaging", ".execution", ".services"))
        ]
    if relative.endswith("/services.py"):
        return [
            d
            for d in dependencies
            if d.startswith(("fastapi", "starlette", "api", "main", "worker", "cli"))
        ]
    if relative.endswith("/models.py"):
        return [
            d
            for d in dependencies
            if d.startswith(("fastapi", "starlette", "api", "main", "worker", "cli"))
            or d.endswith((".services", ".schemas", ".execution", ".messaging"))
        ]
    if relative.endswith("/schemas.py") or relative.endswith("/contracts.py"):
        return [
            d
            for d in dependencies
            if d.startswith(
                ("sqlalchemy", "celery", "kombu", "api", "db", "worker", "cli", "httpx")
            )
            or d.endswith((".models", ".services", ".execution", ".messaging"))
        ]
    if relative.startswith("core/"):
        return [
            d for d in dependencies if d.split(".")[0] in PACKAGES and not d.startswith("core.")
        ]
    return []


def cross_domain_model_imports(relative, tree):
    if not relative.endswith("/services.py"):
        return []
    owner = relative.split("/", 1)[0]
    return [
        dependency
        for dependency in imports(tree)
        if dependency.endswith(".models") and dependency.split(".", 1)[0] != owner
    ]


def test_import_boundaries_and_no_hidden_initializer_logic():
    violations = []
    for path in application_files():
        relative = path.relative_to(SOURCE).as_posix()
        tree = ast.parse(path.read_text())
        violations.extend(f"{relative}: forbidden {d}" for d in forbidden_imports(relative, tree))
        violations.extend(
            f"{relative}: cross-domain ORM import {d}"
            for d in cross_domain_model_imports(relative, tree)
        )
        if path.name == "__init__.py":
            assert all(
                isinstance(n, ast.Expr)
                and isinstance(n.value, ast.Constant)
                and isinstance(n.value.value, str)
                for n in tree.body
            ), relative
        if relative.startswith("api/routers/"):
            assert not any(
                isinstance(n, ast.Attribute)
                and n.attr
                in {
                    "database",
                    "sessions",
                    "execute",
                    "commit",
                    "publish",
                    "send_task",
                    "delay",
                    "apply_async",
                }
                for n in ast.walk(tree)
            ), relative
            assert not any(isinstance(n, ast.AsyncFunctionDef) for n in tree.body), (
                "Blocking SQLAlchemy routes must use def: " + relative
            )
        for node in ast.walk(tree):
            if isinstance(node, ast.ImportFrom):
                assert not node.level, f"Use absolute imports: {relative}"
        assert re.fullmatch(r"[a-z0-9_]+\.py", path.name), relative
    assert not violations, "\n".join(violations)


@pytest.mark.parametrize(
    ("module", "dependency"),
    [
        ("api/routers/monitors.py", "sqlalchemy"),
        ("api/routers/jobs.py", "worker.messaging"),
        ("jobs/services.py", "worker.app"),
        ("jobs/schemas.py", "cli.commands"),
        ("identity/services.py", "fastapi"),
        ("monitors/models.py", "monitors.services"),
        ("jobs/schemas.py", "jobs.models"),
        ("core/config.py", "identity.services"),
        ("sources/adapters/bluesky.py", "db.session"),
        ("sources/adapters/bluesky.py", "worker.app"),
        ("evidence/adapters/minio.py", "db.session"),
        ("evidence/adapters/minio.py", "worker.app"),
        ("sources/schemas.py", "httpx"),
        ("ai/adapters/ollama.py", "knowledge.models"),
    ],
)
def test_boundaries_reject_invalid_examples(module, dependency):
    assert forbidden_imports(module, ast.parse(f"import {dependency}")) == [dependency]


def test_services_reject_cross_domain_orm_examples():
    tree = ast.parse("from contents import models")
    assert cross_domain_model_imports("events/services.py", tree) == ["contents.models"]
    assert cross_domain_model_imports("contents/services.py", tree) == []


@pytest.mark.parametrize("reserved", ["reporting"])
def test_future_modules_require_explicit_registration(reserved):
    assert unregistered_top_level_modules([SOURCE / reserved / "module.py"]) == {reserved}


def test_application_import_graph_is_acyclic():
    modules = {}
    for path in application_files():
        if "migrations/versions" in path.as_posix():
            continue
        name = ".".join(path.relative_to(SOURCE).with_suffix("").parts)
        modules[name] = ast.parse(path.read_text())
    graph = {name: [d for d in imports(tree) if d in modules] for name, tree in modules.items()}
    visited = set()
    active = set()

    def visit(name):
        assert name not in active, f"Cyclic dependency at {name}"
        if name in visited:
            return
        active.add(name)
        for child in graph[name]:
            visit(child)
        active.remove(name)
        visited.add(name)

    for name in graph:
        visit(name)


def test_only_main_constructs_fastapi():
    owners = []
    for path in application_files():
        tree = ast.parse(path.read_text())
        for node in ast.walk(tree):
            if (
                isinstance(node, ast.Call)
                and isinstance(node.func, ast.Name)
                and node.func.id == "FastAPI"
            ):
                owners.append(path.relative_to(SOURCE).as_posix())
            if isinstance(node, ast.Call) and isinstance(node.func, ast.Attribute):
                assert node.func.attr != "on_event", (
                    "Use FastAPI lifespan, not startup/shutdown hooks"
                )
    assert owners == ["main.py"]
