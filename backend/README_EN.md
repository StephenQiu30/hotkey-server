# HotKey Server

<p align="center"><a href="README.md">简体中文</a> · <a href="README_EN.md">English</a></p>

<p align="center">
  <strong>Turn scattered public signals into verifiable, traceable, and publishable event intelligence.</strong>
</p>

<p align="center">
  <a href="https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <a href="https://go.dev/"><img alt="Go 1.26+" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white"></a>
  <a href="../LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/license-MIT-green.svg"></a>
  <img alt="Self-hosted" src="https://img.shields.io/badge/deployment-self--hosted-6f42c1">
</p>

HotKey is a local-first, self-hosted platform for AI-assisted trend monitoring and Obsidian knowledge governance. It collects content from compliant sources such as RSS, Atom, and Hacker News, groups cross-source signals into events, preserves the underlying evidence, and produces daily or weekly intelligence reports.

`backend/` is the backend and OpenAPI source of truth for collection, normalization, relevance, event intelligence, reports, delivery, identity, authorization, and operations.

> If HotKey is useful for your research, editorial, or intelligence workflow, consider starring the project, sharing your use case, or contributing through an Issue or Pull Request.

## Repository projects

HotKey keeps two projects in one repository. They can be worked on independently and deployed together:

| Directory | Responsibility |
|------------|----------------|
| [`backend/`](.) | Backend APIs, collection and AI jobs, data models, delivery, and the OpenAPI contract |
| [`frontend/`](../frontend/README_EN.md) | The user-facing workspace for events, monitors, sources, evidence, reports, and settings |

The backend can run the API and background jobs on its own. Run `frontend/` alongside it for the complete interactive experience.

## Why HotKey

- **Local first** — PostgreSQL stores business facts, MinIO stores raw evidence, and your Obsidian Vault receives readable knowledge artifacts.
- **Evidence first** — events, claims, sources, metrics, and AI runs remain traceable instead of treating model output as ground truth.
- **Compliant collection** — connectors use official APIs, public RSS/Atom, or authorized feeds and do not bypass access controls.
- **Human-in-the-loop AI** — AI can expand queries, create embeddings, extract entities and claims, and draft summaries while approvals remain explicit.
- **End-to-end delivery** — monitoring, collection, clustering, frozen reports, Vault publishing, email, and private RSS/Atom live in one workflow.
- **Small-team operations** — one repository, one binary, and a modular monolith designed for individuals and teams of roughly 5–10 people.

## Capabilities

| Area | Implemented capabilities |
|------|--------------------------|
| Identity | Registration, login, session refresh, password reset, roles, and administration |
| Monitoring | Versioned drafts, preview, publish, pause, resume, archive, and AI rule candidate approval |
| Sources | RSS, Atom, Hacker News, health checks, collection runs, and atomic retry across runs, checkpoints, and River jobs |
| Content | Normalization, deduplication, Markdown document view, and MinIO evidence |
| Relevance | Multilingual matching, human feedback, evaluation, and rule suggestions |
| Hotspot monitoring | Event change timelines, explainable Radar, multi-dimensional filtering and sorting, low-noise inbox alerts, and action audits |
| Events | Clustering, lifecycle governance, heat/trend metrics, entities, claims, and evidence-backed summaries |
| AI | OpenAI, DeepSeek, Ollama, and optional local ONNX embeddings |
| Knowledge | Obsidian proposals, approval, reconciliation, daily/weekly reports, and publishing |
| Delivery & Ops | SMTP, private RSS/Atom, reliable jobs, Prometheus, OpenTelemetry, and operations APIs |

Development mode also includes a self-hosted Swagger UI at `/docs` and an OpenAPI document at `/openapi.json`.

The monitoring loop is event-centric rather than article-centric. Every 24-hour heat recomputation deterministically decides whether to append an immutable EventUpdate. Radar returns at most 100 explainable events ranked by attention, momentum, breadth, recency, or Monitor relevance. Material changes that meet a published Monitor threshold enter a low-noise Alert Thread. Alerts support acknowledge, resolve, suppress, optimistic concurrency, and immutable audits; retries and concurrent workers do not duplicate facts. If no AI Provider is configured or a Provider is temporarily unavailable, change detection, Radar, and alerts continue to work while summaries explicitly fall back to evidence-based degraded output.

The authenticated API surface includes `GET /api/v1/radar/events`, `GET /api/v1/events/{id}/updates`, `GET /api/v1/alerts`, `GET /api/v1/alerts/{id}`, and the three alert state actions. OpenAPI is the source of truth for request and response details.

## Architecture

```mermaid
flowchart LR
    A["RSS / Atom / HN"] --> B["Collect and normalize"]
    B --> C["Relevance and deduplication"]
    C --> D["Event clustering and heat"]
    D --> E["Change timeline and Radar"]
    E --> F["Low-noise alerts"]
    E --> G["Entities, claims, summaries"]
    G --> H["Daily / weekly reports"]
    H --> I["Obsidian Vault"]
    H --> J["Email / RSS / Atom"]
    B -. "raw evidence" .-> K["MinIO"]
    B -. "business facts" .-> L["PostgreSQL + pgvector"]
```

The same Go binary can run as `all`, `api`, or `worker`.

## Quick start

### Requirements

- Go 1.26+
- PostgreSQL 16+ with pgvector
- Redis 7+
- MinIO
- Optional: SMTP, OpenAI / DeepSeek, Ollama, ONNX Runtime

### Start with Docker Compose

Docker Compose is the only full-project runtime entry point. The root default Compose file starts the frontend, backend, Python Agent, and infrastructure together. Do not run the Go service directly from the host. The default environment uses `backend/.env`:

```bash
cd ..
cp backend/.env.example backend/.env
cp .env.example .env
docker compose -f docker-compose.yml up --build -d
```

Production uses the root `.env.prod`; fill its six required empty values first:

```bash
cp .env.prod.example .env.prod
docker compose --env-file .env.prod -f docker-compose-prod.yml up --build -d
```

The baseline defines services, initialization, dependencies, health checks, and volumes once. The daily override publishes the Web, Server, PostgreSQL, Redis, and MinIO ports; the production override publishes only Web and Server. `--env-file .env.prod` maps the required container credentials and port overrides. Upstream images use floating `latest` tags; pgvector does not publish `latest`, so it uses the floating `pg16` tag to remain compatible with existing data volumes. Put an HTTPS reverse proxy in front of the production HTTP ports.

### Configure and initialize

```bash
git clone https://github.com/StephenQiu30/hotkey-server.git
cd hotkey-server/backend
cp .env.example .env
```

Configure a dedicated PostgreSQL database, MinIO, Redis, explicit CORS origins, and unique JWT/HMAC secrets of at least 32 bytes. [`.env.example`](.env.example) lists the common settings and optional providers; omitted settings use application defaults. Never commit real credentials.

Registration and password reset require a compatible SMTP service. To enable them, use [`.env.example`](.env.example) to configure the provider host, TLS mode, account, authorization code, and sender. See the [operations guides](../docs/operations/README.md) for deployment and diagnostics.

The Compose `db-init` one-shot service initializes and verifies an empty database before the API receives traffic. Start the complete project from the repository root:

```bash
docker compose -f docker-compose.yml up --build --detach --wait --wait-timeout 240
```

The first user completes normal email verification and registration as a viewer. A database operator then promotes that user through a reviewed, auditable one-time transaction. The rebuilt documentation baseline has not yet accepted this production procedure; before a production deployment, add and rehearse a dedicated runbook through the [operations index](../docs/operations/README.md). Startup does not read an administrator email or password and requires no separate user-bootstrap command.

Verify the runtime:

```bash
curl --fail http://127.0.0.1:8866/healthz
curl --fail http://127.0.0.1:8866/readyz
```

- Swagger UI: <http://127.0.0.1:8866/docs>
- OpenAPI: <http://127.0.0.1:8866/openapi.json>
- Prometheus: <http://127.0.0.1:8866/metrics>

For production, set `HOTKEY_ENV=production` to load `.env.prod` as an override. Process environment variables always take precedence. API documentation routes are disabled in production.

### Host-side debugging

Use GoLand and the installed Go toolchain for formatting, compilation, unit/integration tests, and test debugging, not as a project-service startup path. Tests reuse existing disposable PostgreSQL and Redis services. Only fresh-container E2E, release, and recovery gates create isolated Compose stacks.

## Web application

Use [hotkey-web](https://github.com/StephenQiu30/hotkey-web) for the complete browser workspace, including events, monitors, sources, evidence, reports, and notification settings. Root Compose starts and deploys the backend, Agent, and Web application together.

## Development

The backend is a single-module, single-binary modular monolith:

```text
cmd/hotkey/                # Executable and CLI entry point
internal/bootstrap/        # Fx composition, roles, and lifecycle
internal/modules/          # Domain-oriented application, domain, infrastructure, and HTTP layers
internal/platform/         # Database repositories, HTTP, queue, configuration, logging, and observability adapters
internal/shared/           # Stable errors, pagination/repository contracts, and shared primitives
db/                        # Canonical full schema and embedded resources
test/                      # Centralized tests, mirrored testdata, runner, and architecture gates
../docs/           # Design, PRD, Plan, Acceptance, Operations, and OpenAPI JSON
openapi/                   # Runtime OpenAPI registry
tools/                     # Build-time tool dependencies
```

Business code stays under `internal/` so it cannot be imported outside this module. There is no `pkg/` directory because the repository currently exposes no reusable public Go library. The `all`, `api`, and `worker` values remain runtime roles of the single `cmd/hotkey` entry point.

`internal/shared` does not depend on an ORM, database driver, or `internal/platform`. GORM CRUD and PostgreSQL error mapping live in `internal/platform/database/repository`, while module infrastructure depends inward on the shared contracts.

Alibaba's layering guidance is mapped by responsibility rather than by copying Java package names: HTTP Transport corresponds to Web/Controller, Application to use-case Services, Domain owns business objects, entities, value objects, and ports, unexported Infrastructure/PostgreSQL records correspond to DOs, and HTTP RequestDTO/ResponseDTO types define the presentation boundary. Application code never imports a concrete PostgreSQL repository; adapters implement Domain ports, and `TestModuleLayersKeepInwardDependencies` protects that direction. This preserves idiomatic Go packages while honoring the POJO principle that plain contracts carry no framework or persistence details.

Test sources and test-only fixtures live in package-mirrored paths below `test/_suite`. During execution, `test/runner` temporarily materializes `_test.go` files and package-local `testdata`, then removes those links, keeping test-only assets out of the `internal/` production tree.

This layout is an evidence-based choice rather than a copy of a purported standard template:

- [golang-standards/project-layout](https://github.com/golang-standards/project-layout) explicitly says it is not an official Go standard and recommends keeping only what a project needs.
- [ardanlabs/service](https://github.com/ardanlabs/service) structures a production starter around application, business, and foundation layers without requiring the same names used here.
- [ThreeDotsLabs/wild-workouts-go-ddd-example](https://github.com/ThreeDotsLabs/wild-workouts-go-ddd-example/tree/master/internal) organizes application, domain, ports, and adapters within business capabilities, which is the closest match to HotKey's module boundaries.
- [Grafana](https://github.com/grafana/grafana/tree/main/pkg/services), [Gitea](https://github.com/go-gitea/gitea/tree/main/services), and [Prometheus](https://github.com/prometheus/prometheus) use different service layouts, demonstrating that large Go projects do not converge on one directory tree.

The shared GoLand configuration follows the [official JetBrains Run/Debug configuration model](https://www.jetbrains.com/help/go/run-debug-configuration.html): it uses the `DIRECTORY` run kind with `$PROJECT_DIR$/cmd/hotkey` as the run directory and the repository root as the working directory, without depending on a developer-specific project module name.

```bash
make openapi
make vet
make test
make build
make architecture repository
```

The complete CI suite requires a disposable PostgreSQL database and a dedicated Redis test database:

```bash
export HOTKEY_TEST_DSN='postgres://hotkey:hotkey@localhost:5432/hotkey_test?sslmode=disable'
export HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15'
make ci
```

`make ci` is the shared backend acceptance entry point for local development and GitHub Actions. It checks OpenAPI drift, database behavior, tests, builds, schemas, architecture boundaries, and production dependency vulnerabilities.

## Project status

HotKey is under active development. The core end-to-end workflow is implemented, but APIs and deployment details may still change before 1.0. It is best suited for technical previews, self-hosted evaluation, and collaborative development rather than unattended critical production workloads.

Long-lived evidence for the rebuilt baseline belongs in the [acceptance index](../docs/acceptance/README.md); no acceptance file currently proves that the new baseline is implemented. Roadmap and feature proposals are discussed publicly in [GitHub Issues](https://github.com/StephenQiu30/hotkey-server/issues). External dependencies, backups, and restores must still be exercised in each deployment environment using the [operations guides](../docs/operations/README.md).

## Documentation

- [Architecture and design](../docs/design/README.md)
- [Product requirements](../docs/prd/README.md)
- [Implementation plans](../docs/plans/README.md)
- [Acceptance evidence](../docs/acceptance/README.md)
- [Operations guides](../docs/operations/README.md)
- [OpenAPI JSON](../docs/openapi/swagger.json)

## Contributing

Bug reports, use cases, connector proposals, documentation, and code contributions are welcome. Read the [contribution guide](../CONTRIBUTING.md) and [security policy](../SECURITY.md) before getting started.

Please open an Issue before large changes. New connectors must document an official or authorized access path.

## License

HotKey Server is open source under the repository-level [MIT License](../LICENSE).
