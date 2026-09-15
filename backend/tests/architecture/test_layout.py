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
