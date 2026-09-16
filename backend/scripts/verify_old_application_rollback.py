"""Verify an old application can serve reads from an isolated pre-release database."""

import io
import json
import os
import re
import socket
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request
from collections.abc import Iterator
from contextlib import contextmanager
from http.cookies import SimpleCookie
from pathlib import Path
from uuid import uuid4

from sqlalchemy import create_engine, inspect, text
from sqlalchemy.engine import URL, make_url

BASELINE_COMMIT = "13545f223ea9257464c6d4f8c91c16c2543eb8d0"
BASELINE_REVISION = "0019_collection_retry_budget"
CURRENT_REVISION = "0020_trend_alerts"


def compose(*arguments: str, timeout: int = 30) -> str:
    return subprocess.check_output(
        ["docker", "compose", *arguments], text=True, timeout=timeout
    ).strip()


def validate_target() -> URL:
    project = os.getenv("COMPOSE_PROJECT_NAME", "")
    if not re.fullmatch(r"hotkey-[a-z0-9-]+", project) or project == "hotkey":
        raise RuntimeError("rollback_poc_requires_disposable_compose_project")
    raw_url = os.getenv("HOTKEY_ROLLBACK_POC_DATABASE_URL")
    if not raw_url:
        raise RuntimeError("rollback_poc_database_url_required")
    url = make_url(raw_url)
    if (
        url.drivername != "postgresql+psycopg"
        or url.database != "hotkey"
        or url.host not in {"127.0.0.1", "localhost"}
    ):
        raise RuntimeError("rollback_poc_requires_loopback_hotkey_database")
    container = compose("ps", "-q", "postgres", timeout=10)
    if not container:
        raise RuntimeError("rollback_poc_postgres_not_running")
    actual_project = subprocess.check_output(
        [
            "docker",
            "inspect",
            "--format",
            '{{ index .Config.Labels "com.docker.compose.project" }}',
            container,
        ],
        text=True,
        timeout=10,
    ).strip()
    if actual_project != project:
        raise RuntimeError("rollback_poc_compose_project_mismatch")
    subprocess.run(
        ["git", "cat-file", "-e", f"{BASELINE_COMMIT}^{{commit}}"],
        check=True,
        timeout=10,
    )
    return url


def database_state(url: URL) -> dict[str, object]:
    engine = create_engine(url)
    try:
        table_names = sorted(inspect(engine).get_table_names(schema="public"))
        with engine.connect() as connection:
            revision = connection.scalar(text("SELECT version_num FROM alembic_version"))
            counts = {
                name: int(
                    connection.scalar(
                        text(f'SELECT count(*) FROM "{name.replace(chr(34), chr(34) * 2)}"')
                    )
                    or 0
                )
                for name in table_names
            }
        return {"revision": revision, "table_counts": counts}
    finally:
        engine.dispose()


def export_baseline(destination: Path) -> Path:
    archive = subprocess.check_output(
        ["git", "archive", "--format=tar", BASELINE_COMMIT, "backend"], timeout=30
    )
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:") as source:
        source.extractall(destination, filter="data")
    source_root = destination / "backend" / "src"
    if not (source_root / "main.py").is_file():
        raise RuntimeError("rollback_poc_baseline_export_missing")
    return source_root


def application_environment(database_url: URL, source_root: Path) -> dict[str, str]:
    environment = os.environ.copy()
    environment.update(
        {
            "HOTKEY_DATABASE_URL": database_url.render_as_string(hide_password=False),
            "HOTKEY_BROKER_URL": os.getenv(
                "HOTKEY_TEST_BROKER_URL",
                "amqp://hotkey:learning@127.0.0.1:15673/hotkey",
            ),
            "HOTKEY_ALLOWED_ORIGINS": '["http://127.0.0.1:8010"]',
            "HOTKEY_SOURCE_RIGHTS_ALLOWED": "[]",
            "HOTKEY_SOURCE_PIPELINES_CONNECTED": "[]",
            "PYTHONPATH": str(source_root),
        }
    )
    return environment


def run_old_cli(
    source_root: Path,
    database_url: URL,
    *arguments: str,
    input_value: str | None = None,
) -> None:
    subprocess.run(
        [sys.executable, "-m", "cli", *arguments],
        cwd=source_root,
        env=application_environment(database_url, source_root),
        input=input_value,
        text=True,
        check=True,
        timeout=60,
    )


def free_port() -> int:
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return int(listener.getsockname()[1])


@contextmanager
def old_application(source_root: Path, database_url: URL) -> Iterator[str]:
    port = free_port()
    process = subprocess.Popen(
        [
            sys.executable,
            "-m",
            "uvicorn",
            "main:create_app",
            "--factory",
            "--host",
            "127.0.0.1",
            "--port",
            str(port),
            "--no-access-log",
        ],
        cwd=source_root,
        env=application_environment(database_url, source_root),
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    try:
        base_url = f"http://127.0.0.1:{port}"
        deadline = time.monotonic() + 30
        while process.poll() is None and time.monotonic() < deadline:
            try:
                status, _, _ = request_json(f"{base_url}/health/live")
                if status == 200:
                    yield base_url
                    return
            except urllib.error.URLError:
                pass
            time.sleep(0.25)
        raise RuntimeError("rollback_poc_old_application_start_failed")
    finally:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)


def request_json(
    url: str,
    *,
    method: str = "GET",
    payload: dict[str, object] | None = None,
    cookie: str | None = None,
) -> tuple[int, object, list[str]]:
    data = None if payload is None else json.dumps(payload).encode()
    headers = {"Content-Type": "application/json"} if data is not None else {}
    if method not in {"GET", "HEAD", "OPTIONS"}:
        headers["Origin"] = "http://127.0.0.1:8010"
    if cookie is not None:
        headers["Cookie"] = cookie
    request = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=5) as response:
            body = response.read()
            return (
                response.status,
                json.loads(body) if body else None,
                response.headers.get_all("Set-Cookie") or [],
            )
    except urllib.error.HTTPError as error:
        body = error.read()
        return (
            error.code,
            json.loads(body) if body else None,
            error.headers.get_all("Set-Cookie") or [],
        )


def session_cookie(headers: list[str]) -> str:
    cookies = SimpleCookie()
    for header in headers:
        cookies.load(header)
    if "hk_session" not in cookies:
        raise RuntimeError("rollback_poc_session_cookie_missing")
    return f"hk_session={cookies['hk_session'].value}"


def create_read_only_role(database: str, role: str, password: str) -> None:
    statements = (
        f"CREATE ROLE {role} LOGIN PASSWORD '{password}'",
        f"ALTER ROLE {role} SET default_transaction_read_only = on",
        f"GRANT CONNECT ON DATABASE {database} TO {role}",
    )
    for statement in statements:
        compose(
            "exec",
            "-T",
            "postgres",
            "psql",
            "-U",
            "hotkey",
            "-d",
            "postgres",
            "-v",
            "ON_ERROR_STOP=1",
            "-c",
            statement,
        )
    for statement in (
        f"GRANT USAGE ON SCHEMA public TO {role}",
        f"GRANT SELECT ON ALL TABLES IN SCHEMA public TO {role}",
        f"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO {role}",
    ):
        compose(
            "exec",
            "-T",
            "postgres",
            "psql",
            "-U",
            "hotkey",
            "-d",
            database,
            "-v",
            "ON_ERROR_STOP=1",
            "-c",
            statement,
        )


def drop_role(role: str) -> None:
    compose(
        "exec",
        "-T",
        "postgres",
        "psql",
        "-U",
        "hotkey",
        "-d",
        "postgres",
        "-v",
        "ON_ERROR_STOP=1",
        "-c",
        f"DROP ROLE IF EXISTS {role}",
    )


def main() -> None:
    source_url = validate_target()
    suffix = uuid4().hex[:12]
    snapshot_database = f"hotkey_rollback_{suffix}"
    read_role = f"hotkey_reader_{suffix}"
    read_password = f"RollbackRead{suffix}"
    username = f"rollback-{suffix}"
    owner_password = f"RollbackOwner{suffix}!"
    source_before = database_state(source_url)
    if source_before["revision"] != CURRENT_REVISION:
        raise RuntimeError("rollback_poc_current_schema_revision_mismatch")
    result: dict[str, object] = {}
    with tempfile.TemporaryDirectory(prefix="hotkey-old-app-") as temporary:
        source_root = export_baseline(Path(temporary))
        snapshot_url = source_url.set(database=snapshot_database)
        read_url = snapshot_url.set(username=read_role, password=read_password)
        try:
            with old_application(source_root, source_url) as old_current:
                mismatch_status, mismatch_body, _ = request_json(f"{old_current}/health/ready")
            if mismatch_status != 503 or mismatch_body != {
                "status": "not_ready",
                "code": "schema_mismatch",
            }:
                raise RuntimeError("rollback_poc_expected_schema_mismatch_missing")

            compose("exec", "-T", "postgres", "createdb", "-U", "hotkey", snapshot_database)
            run_old_cli(source_root, snapshot_url, "migrate")
            run_old_cli(
                source_root,
                snapshot_url,
                "owner-init",
                username,
                "--password-stdin",
                input_value=owner_password + "\n",
            )
            with old_application(source_root, snapshot_url) as writable_old:
                login_status, login_body, headers = request_json(
                    f"{writable_old}/api/session",
                    method="POST",
                    payload={"username": username, "password": owner_password},
                )
            if login_status != 200 or login_body != {"username": username}:
                raise RuntimeError("rollback_poc_old_login_failed")
            cookie = session_cookie(headers)

            create_read_only_role(snapshot_database, read_role, read_password)
            snapshot_before = database_state(snapshot_url)
            if snapshot_before["revision"] != BASELINE_REVISION:
                raise RuntimeError("rollback_poc_baseline_schema_revision_mismatch")
            snapshot_table_counts = snapshot_before.get("table_counts")
            if not isinstance(snapshot_table_counts, dict):
                raise RuntimeError("rollback_poc_baseline_table_counts_missing")
            read_engine = create_engine(read_url)
            try:
                with read_engine.connect() as connection:
                    read_only = connection.scalar(text("SHOW transaction_read_only"))
            finally:
                read_engine.dispose()
            if read_only != "on":
                raise RuntimeError("rollback_poc_read_only_role_not_enforced")

            with old_application(source_root, read_url) as read_only_old:
                ready_status, ready_body, _ = request_json(f"{read_only_old}/health/ready")
                session_status, session_body, _ = request_json(
                    f"{read_only_old}/api/session", cookie=cookie
                )
                events_status, events_body, _ = request_json(
                    f"{read_only_old}/api/events?limit=20", cookie=cookie
                )
            snapshot_after = database_state(snapshot_url)
            source_after = database_state(source_url)
            if ready_status != 200 or ready_body != {
                "status": "ready",
                "scope": "database_schema",
            }:
                raise RuntimeError("rollback_poc_old_application_not_ready")
            if session_status != 200 or session_body != {"username": username}:
                raise RuntimeError("rollback_poc_read_only_session_failed")
            if events_status != 200 or not isinstance(events_body, dict):
                raise RuntimeError("rollback_poc_read_only_events_failed")
            if snapshot_after != snapshot_before:
                raise RuntimeError("rollback_poc_snapshot_changed_during_reads")
            if source_after != source_before:
                raise RuntimeError("rollback_poc_current_database_changed")
            result = {
                "scope": "old_application_isolated_snapshot_rollback",
                "baseline_commit": BASELINE_COMMIT,
                "baseline_revision": BASELINE_REVISION,
                "current_revision": CURRENT_REVISION,
                "direct_current_database": {
                    "status": mismatch_status,
                    "code": "schema_mismatch",
                },
                "isolated_snapshot": {
                    "ready": ready_status == 200,
                    "authenticated_read": session_status == 200,
                    "event_list_read": events_status == 200,
                    "transaction_read_only": read_only,
                    "table_count": len(snapshot_table_counts),
                    "counts_unchanged": snapshot_after == snapshot_before,
                },
                "current_database_unchanged": source_after == source_before,
            }
        finally:
            compose(
                "exec",
                "-T",
                "postgres",
                "dropdb",
                "-U",
                "hotkey",
                "--if-exists",
                "--force",
                snapshot_database,
            )
            drop_role(read_role)
    databases = compose(
        "exec",
        "-T",
        "postgres",
        "psql",
        "-U",
        "hotkey",
        "-d",
        "postgres",
        "-Atc",
        f"SELECT count(*) FROM pg_database WHERE datname='{snapshot_database}'",
    )
    roles = compose(
        "exec",
        "-T",
        "postgres",
        "psql",
        "-U",
        "hotkey",
        "-d",
        "postgres",
        "-Atc",
        f"SELECT count(*) FROM pg_roles WHERE rolname='{read_role}'",
    )
    if databases != "0" or roles != "0":
        raise RuntimeError("rollback_poc_cleanup_failed")
    result["cleanup"] = {
        "snapshot_database_removed": True,
        "read_only_role_removed": True,
        "temporary_source_removed": True,
    }
    print(json.dumps(result, sort_keys=True))


if __name__ == "__main__":
    main()
