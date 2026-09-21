FROM ghcr.io/astral-sh/uv:0.11.32 AS uv
FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy

COPY --from=uv /uv /uvx /bin/

WORKDIR /srv/backend
COPY pyproject.toml uv.lock ./
RUN uv sync --frozen --no-dev

COPY database ./database
COPY app ./app

RUN useradd --create-home --uid 10001 hotkey \
    && chown -R hotkey:hotkey /srv/backend

USER hotkey
WORKDIR /srv/backend/app
EXPOSE 8080

CMD ["/srv/backend/.venv/bin/uvicorn", "main:create_app", "--factory", "--host", "0.0.0.0", "--port", "8080"]
