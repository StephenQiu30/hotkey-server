from __future__ import annotations

import ast
from pathlib import Path

from api.router import api_router
from core.errors import ERROR_CATEGORIES, ApplicationError

BACKEND = Path(__file__).resolve().parents[2]
APP = BACKEND / "app"


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
    assert not (BACKEND / "src").exists()
    assert not (APP / "src").exists()
    assert not (APP / "app").exists()
    assert not (APP / "hotkey").exists()
    assert not any(path.name.startswith("v") and path.name[1:].isdigit() for path in APP.rglob("*"))


def test_backend_has_no_empty_top_level_package_placeholders() -> None:
    for package in APP.iterdir():
        if not package.is_dir() or package.name == "__pycache__":
            continue
        implementation_files = [
            path
            for path in package.rglob("*.py")
            if path.name != "__init__.py" and "__pycache__" not in path.parts
        ]
        assert implementation_files, package


def test_public_api_uses_the_single_stable_namespace() -> None:
    assert api_router.prefix == "/api"


def test_schema_sql_is_the_only_ddl_source() -> None:
    assert (BACKEND / "database" / "schema.sql").is_file()
    assert not (BACKEND / "alembic.ini").exists()
    assert not (BACKEND / "migrations").exists()
    assert "alembic" not in (BACKEND / "pyproject.toml").read_text().lower()


def test_runtime_does_not_create_or_drop_schema() -> None:
    runtime_source = "\n".join(path.read_text() for path in APP.rglob("*.py"))
    assert ".create_all(" not in runtime_source
    assert ".drop_all(" not in runtime_source


def test_routers_do_not_import_persistence_or_service_implementations() -> None:
    for path in (APP / "api" / "routers").glob("*.py"):
        imports = _imports(path)
        assert not any(name.startswith("sqlalchemy") for name in imports)
        assert not any(name.endswith(".models") for name in imports)
        assert not any(name.endswith(".services") for name in imports)


def test_schema_modules_do_not_depend_on_http_or_orm() -> None:
    for path in APP.rglob("schemas.py"):
        imports = _imports(path)
        assert not any(name.startswith(("fastapi", "starlette", "sqlalchemy")) for name in imports)


def test_source_contracts_do_not_depend_on_runtime_or_business_domains() -> None:
    imports = _imports(APP / "sources" / "contracts.py")
    forbidden = (
        "api",
        "db",
        "evidence",
        "jobs",
        "worker",
        "fastapi",
        "httpx",
        "sqlalchemy",
        "starlette",
    )

    assert not any(
        name == prefix or name.startswith(f"{prefix}.") for name in imports for prefix in forbidden
    )


def test_application_errors_are_registered_without_http_status() -> None:
    error = ApplicationError(next(iter(ERROR_CATEGORIES)))

    assert error.code in ERROR_CATEGORIES
    assert not hasattr(error, "status_code")
    assert not hasattr(error, "message")
    assert not any(
        name.startswith(("fastapi", "starlette")) for name in _imports(APP / "core" / "errors.py")
    )


def test_worker_does_not_depend_on_http_protocol() -> None:
    for path in (APP / "worker").glob("*.py"):
        assert not any(name.startswith(("fastapi", "starlette")) for name in _imports(path))


def test_resource_routes_declare_openapi_contract_fields() -> None:
    http_methods = {"get", "post", "put", "patch", "delete", "options", "head", "trace"}
    for path in (APP / "api" / "routers").glob("*.py"):
        tree = ast.parse(path.read_text())
        for node in ast.walk(tree):
            if not isinstance(node, ast.Call) or not isinstance(node.func, ast.Attribute):
                continue
            if node.func.attr not in http_methods:
                continue
            keyword_names = {keyword.arg for keyword in node.keywords if keyword.arg is not None}
            assert {"operation_id", "response_model", "status_code"} <= keyword_names, path


def test_resource_routers_declare_openapi_tags() -> None:
    for path in (APP / "api" / "routers").glob("*.py"):
        if path.name == "__init__.py":
            continue
        tree = ast.parse(path.read_text())
        router_calls = [
            node
            for node in ast.walk(tree)
            if isinstance(node, ast.Call)
            and isinstance(node.func, ast.Name)
            and node.func.id == "APIRouter"
        ]
        assert router_calls, path
        assert all(
            any(keyword.arg == "tags" for keyword in call.keywords) for call in router_calls
        ), path
