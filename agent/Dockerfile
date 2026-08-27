FROM python:3.12-slim AS builder

ENV PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1 \
    UV_PROJECT_ENVIRONMENT=/opt/venv

WORKDIR /build
RUN python -m pip install uv==0.11.32
COPY pyproject.toml uv.lock README.md ./
COPY src ./src
RUN uv sync --frozen --no-dev --no-editable

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH=/opt/venv/bin:${PATH}

RUN groupadd --system hotkey \
    && useradd --system --gid hotkey --create-home --home-dir /home/hotkey hotkey

COPY --from=builder /opt/venv /opt/venv
USER hotkey
WORKDIR /home/hotkey

EXPOSE 8090
CMD ["uvicorn", "hotkey_agent.main:app", "--host", "0.0.0.0", "--port", "8090", "--no-access-log"]
