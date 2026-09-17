import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


def test_canonical_backend_layout():
    assert (ROOT / "backend/src/main.py").is_file()
    assert not (ROOT / "server").exists()
    assert (ROOT / "backend/src/worker/app.py").is_file()
    assert (ROOT / "backend/src/worker/messaging.py").is_file()
    assert (ROOT / "backend/src/cli/__main__.py").is_file()
    for legacy in ("hotkey", "app", "workspace"):
        assert not (ROOT / "backend/src" / legacy).exists()
    for legacy in ("main.py", "worker.py", "cli.py", "api", "jobs"):
        assert not (ROOT / "backend" / legacy).exists()
    assert not (ROOT / "backend/workspace").exists()


def test_source_io_uses_the_registered_adapter_directory():
    assert (ROOT / "backend/src/sources/adapters/bluesky.py").is_file()
    assert not (ROOT / "backend/src/sources/bluesky.py").exists()


def test_deployment_uses_canonical_backend_entrypoint():
    compose = (ROOT / "docker-compose.yml").read_text()
    assert "\n  backend:\n" in compose and "\n  api:\n" not in compose
    assert "build: ./backend" in compose and "build: ./server" not in compose
    assert "main:create_app" in compose
    assert "worker.app:app" in compose
    assert "target: development" in compose
    assert "develop:" in compose
    assert "action: sync+restart" in compose
    assert "--reload" in compose
    assert "COPY --chown=10001:10001 src ./src" in (ROOT / "backend/Dockerfile").read_text()
    assert "HOTKEY_API_ORIGIN: ${HOTKEY_API_ORIGIN:-http://backend:8080}" in compose
    frontend_dockerfile = (ROOT / "frontend/Dockerfile").read_text()
    assert "npm ci --include=dev" in frontend_dockerfile
    assert 'output: "standalone"' in (ROOT / "frontend/next.config.ts").read_text()
    assert "HOTKEY_API_ORIGIN=${HOTKEY_API_ORIGIN}" in frontend_dockerfile


def test_environment_overlay_is_a_protected_test_deployment():
    environment = (ROOT / "docker-compose-env.yml").read_text()
    environment_example = (ROOT / ".env.env.example").read_text()
    for name in (
        "HOTKEY_DATABASE_URL",
        "HOTKEY_BROKER_URL",
        "HOTKEY_ALLOWED_ORIGINS",
        "POSTGRES_PASSWORD",
        "RABBITMQ_PASSWORD",
    ):
        assert name in environment
    assert "HOTKEY_ENVIRONMENT: production" in environment
    assert "HOTKEY_COOKIE_SECURE: \"true\"" in environment
    assert "target: production" in environment
    assert "HOTKEY_BACKEND_IMAGE=hotkey-backend:env" in environment_example
    assert "HOTKEY_WEB_IMAGE=hotkey-web:env" in environment_example
    for service in ("postgres", "rabbitmq", "backend"):
        block = re.search(rf"(?ms)^  {service}:\n(?:(?!^  \S).)*", environment)
        assert block is not None
        assert "ports: !reset []" in block.group()
    for service in ("backend", "worker", "scheduler", "web"):
        block = re.search(rf"(?ms)^  {service}:\n(?:(?!^  \S).)*", environment)
        assert block is not None
        assert "develop: !reset {}" in block.group()


def test_production_compose_hardens_application_containers():
    production = (ROOT / "docker-compose-prod.yml").read_text()
    production_example = (ROOT / ".env.prod.example").read_text()
    assert "target: production" in production
    assert "HOTKEY_BACKEND_IMAGE=hotkey-backend:prod" in production_example
    assert "HOTKEY_WEB_IMAGE=hotkey-web:prod" in production_example
    assert "read_only: true" in production
    assert "cap_drop: [ALL]" in production
    assert "no-new-privileges:true" in production
    assert "/app/.next/cache:rw,noexec,nosuid,size=64m,uid=1000,gid=1000,mode=0755" in production
    for service in ("backend", "worker", "scheduler", "web"):
        block = re.search(rf"(?ms)^  {service}:\n(?:(?!^  \S).)*", production)
        assert block is not None
        assert "develop: !reset {}" in block.group()


def test_application_roles_have_an_explicit_host_gateway_for_existing_services():
    compose = (ROOT / "docker-compose.yml").read_text()
    app = re.search(r"(?ms)^x-app:.*?(?=^services:)", compose)
    assert app is not None
    assert 'extra_hosts: ["host.docker.internal:host-gateway"]' in app.group()


def test_production_roles_receive_storage_and_exact_source_admission_configuration():
    production = (ROOT / "docker-compose-prod.yml").read_text()
    for name in (
        "HOTKEY_S3_ENDPOINT",
        "HOTKEY_S3_ACCESS_KEY",
        "HOTKEY_S3_SECRET_KEY",
        "HOTKEY_S3_BUCKET",
        "HOTKEY_S3_SECURE",
        "HOTKEY_SOURCE_RIGHTS_ALLOWED",
        "HOTKEY_SOURCE_PIPELINES_CONNECTED",
    ):
        assert name in production
    for role in ("migrate", "backend", "worker", "scheduler"):
        block = re.search(rf"(?ms)^  {role}:\n(?:(?!^  \S).)*", production)
        assert block is not None
        assert "environment: *production" in block.group()
