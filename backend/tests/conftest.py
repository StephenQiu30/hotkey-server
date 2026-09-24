import sys
from pathlib import Path

import pytest
from fastapi import FastAPI

from core.config import Settings
from main import create_app

_BACKEND_ROOT = str(Path(__file__).resolve().parent.parent)
if _BACKEND_ROOT not in sys.path:
    sys.path.insert(0, _BACKEND_ROOT)


@pytest.fixture
def app() -> FastAPI:
    settings = Settings(
        environment="test",
        log_level="WARNING",
        database_url="postgresql+psycopg://test:test@127.0.0.1:5432/hotkey_test",
    )
    return create_app(settings)


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"
