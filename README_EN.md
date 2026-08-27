# HotKey

[![CI](https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml)

HotKey keeps its Go Core, Python data-analysis Agent, and Web workspace in one repository.

| Directory | Contents |
| --- | --- |
| [`backend/`](backend/README_EN.md) | Go backend, PostgreSQL schema, collection and AI jobs, runtime OpenAPI, and image configuration |
| [`agent/`](agent/README.md) | Internal Python data-analysis service, versioned contract, and isolated security boundary |
| [`frontend/`](frontend/README_EN.md) | Next.js Web workspace, UI components, and the generated API client |
| [`docs/`](docs/README.md) | Unified formal documentation and the published OpenAPI contract, without app-specific layers |

## Development

Run each project from its own directory:

```bash
git clone https://github.com/StephenQiu30/hotkey-server.git
cd hotkey-server

# Backend
cd backend
cp .env.example .env
go run ./cmd/hotkey

# Python Agent (another terminal; production Go Worker wiring is still pending)
cd agent
uv sync --all-extras --locked
export HOTKEY_AGENT_AUTH_TOKEN=development-agent-token-change-me-000000
uv run uvicorn hotkey_agent.main:app --host 127.0.0.1 --port 8090

# Frontend (in another terminal)
cd frontend
npm ci
npm run dev
```

The backend listens on `http://127.0.0.1:8866`, the Agent is local-only on `http://127.0.0.1:8090`, and the frontend starts at `http://127.0.0.1:8010`. The Agent receives no business-storage or source credentials.

## Docker Compose

The root `docker-compose.yml` defines the frontend, Go Core, internal Python Agent, PostgreSQL, Redis, MinIO, default environment, health checks, and volumes. The Agent publishes no host port. The production file contains only production differences. For the default environment:

```bash
cp backend/.env.example backend/.env
cp .env.example .env
docker compose -f docker-compose.yml up --build --detach --wait --wait-timeout 240
```

For production:

```bash
cp .env.prod.example .env.prod
docker compose --env-file .env.prod -f docker-compose-prod.yml up --build --detach --wait --wait-timeout 240
```

Common services exist only in the baseline; the two overrides do not repeat dependencies or health checks. Subprojects keep no Compose files. Their Dockerfiles and `.dockerignore` files remain beside each application to preserve focused build contexts.

## Quality gates

```bash
cd backend
make openapi  # Regenerate runtime and published OpenAPI after contract changes.
make ci       # Full backend acceptance; requires isolated PostgreSQL and Redis test services.

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build

cd ../agent
uv sync --all-extras --locked
uv run ruff format --check .
uv run ruff check .
uv run mypy src
uv run pytest
uv run pip-audit
```

The published backend OpenAPI contract is [`docs/openapi/swagger.json`](docs/openapi/swagger.json), while runtime registration code lives at `backend/openapi/docs.go`. The generated frontend client lives under `frontend/src/services/hotkey/hotkey-server/`.

The rebuilt deployment, recovery, source-incident, and capacity contracts are indexed in the [operations guides](docs/operations/README.md). A guide marked `planned` has not passed the rebuilt baseline acceptance gate and is not yet a production procedure.

See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md) for contribution and vulnerability-reporting guidance.
