# syntax=docker/dockerfile:1.7
FROM python:3.12-slim
COPY --from=ghcr.io/astral-sh/uv:0.11.32 /uv /usr/local/bin/uv
WORKDIR /app
COPY pyproject.toml uv.lock ./
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy
RUN --mount=type=cache,target=/root/.cache/uv uv sync --locked --no-dev
COPY --chown=10001:10001 src ./src
WORKDIR /app/src
ENV PATH="/app/.venv/bin:$PATH" PYTHONUNBUFFERED=1 PYTHONDONTWRITEBYTECODE=1
RUN groupadd --gid 10001 hotkey && useradd --uid 10001 --gid 10001 --no-create-home --shell /usr/sbin/nologin hotkey
USER hotkey:hotkey
CMD ["uvicorn", "main:create_app", "--factory", "--host", "0.0.0.0", "--port", "8080", "--no-access-log"]
