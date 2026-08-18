# HotKey

[![CI](https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml)

HotKey keeps its backend and Web workspace in one repository.

| Directory | Contents |
| --- | --- |
| [`backend/`](backend/README_EN.md) | Go backend, PostgreSQL schema, collection and AI jobs, runtime OpenAPI, and image configuration |
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

# Frontend (in another terminal)
cd frontend
npm ci
npm run dev
```

The backend listens on `http://127.0.0.1:8866` by default, and the frontend starts at `http://127.0.0.1:8010`. See each project README for environment and deployment details.

## Docker Compose

The root `docker-compose.yml` defines the frontend, backend, PostgreSQL, Redis, MinIO, default environment, health checks, and volumes. The production file contains only production differences. For the default environment:

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
make ci

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build
```

The published backend OpenAPI contract is [`docs/openapi/swagger.json`](docs/openapi/swagger.json), while runtime registration code lives at `backend/openapi/docs.go`. The generated frontend client lives under `frontend/src/services/hotkey/hotkey-server/`.

See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md) for contribution and vulnerability-reporting guidance.
