# HotKey Server

<p align="center"><a href="README.md">简体中文</a> · <a href="README_EN.md">English</a></p>

<p align="center">
  <strong>Turn scattered public signals into verifiable, traceable, and publishable event intelligence.</strong>
</p>

<p align="center">
  <a href="https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <a href="https://go.dev/"><img alt="Go 1.26+" src="https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go&logoColor=white"></a>
  <a href="LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/license-MIT-green.svg"></a>
  <img alt="Self-hosted" src="https://img.shields.io/badge/deployment-self--hosted-6f42c1">
</p>

HotKey is a local-first, self-hosted platform for AI-assisted trend monitoring and Obsidian knowledge governance. It collects content from compliant sources such as RSS, Atom, and Hacker News, groups cross-source signals into events, preserves the underlying evidence, and produces daily or weekly intelligence reports.

`hotkey-server` is the backend and OpenAPI source of truth for collection, normalization, relevance, event intelligence, reports, delivery, identity, authorization, and operations.

> If HotKey is useful for your research, editorial, or intelligence workflow, consider starring the project, sharing your use case, or contributing through an Issue or Pull Request.

## Project repositories

HotKey is developed as two open-source repositories that can be worked on independently and deployed together:

| Repository | Responsibility |
|------------|----------------|
| [hotkey-server](https://github.com/StephenQiu30/hotkey-server) | This repository: backend APIs, collection and AI jobs, data models, delivery, and the OpenAPI contract |
| [hotkey-web](https://github.com/StephenQiu30/hotkey-web) | The user-facing workspace for events, monitors, sources, evidence, reports, and settings |

This repository can run the API and background jobs on its own. Run `hotkey-web` alongside it for the complete interactive experience.

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
| Events | Clustering, lifecycle governance, heat/trend metrics, entities, claims, and evidence-backed summaries |
| AI | OpenAI, DeepSeek, Ollama, and optional local ONNX embeddings |
| Knowledge | Obsidian proposals, approval, reconciliation, daily/weekly reports, and publishing |
| Delivery & Ops | SMTP, private RSS/Atom, reliable jobs, Prometheus, OpenTelemetry, and operations APIs |

Development mode also includes a self-hosted Swagger UI at `/docs` and an OpenAPI document at `/openapi.json`.

## Architecture

```mermaid
flowchart LR
    A["RSS / Atom / HN"] --> B["Collect and normalize"]
    B --> C["Relevance and deduplication"]
    C --> D["Event clustering and heat"]
    D --> E["Entities, claims, summaries"]
    E --> F["Daily / weekly reports"]
    F --> G["Obsidian Vault"]
    F --> H["Email / RSS / Atom"]
    B -. "raw evidence" .-> I["MinIO"]
    B -. "business facts" .-> J["PostgreSQL + pgvector"]
```

The same Go binary can run as `all`, `api`, or `worker`.

## Quick start

### Requirements

- Go 1.26+
- PostgreSQL 16+ with pgvector
- Redis 7+
- MinIO
- Optional: SMTP, OpenAI / DeepSeek, Ollama, ONNX Runtime

### Configure and initialize

```bash
git clone https://github.com/StephenQiu30/hotkey-server.git
cd hotkey-server
cp .env.example .env
```

Configure a dedicated PostgreSQL database, MinIO, Redis, explicit CORS origins, and unique JWT/HMAC secrets of at least 32 bytes. See [`.env.example`](.env.example) for every option and never commit real credentials.

Registration and password reset require a compatible SMTP service. To enable them, use [`.env.example`](.env.example) to configure the provider host, TLS mode, account, authorization code, and sender. See the [operations guides](docs/operations/README.md) for deployment and diagnostics.

Initialize a new, empty database:

```bash
go run ./cmd/hotkey db init --empty-only --confirm-empty
go run ./cmd/hotkey db verify
```

Run the `cmd/hotkey` main package directly in GoLand, or use the single equivalent command-line entry point:

```bash
go run ./cmd/hotkey
```

The first user completes normal email verification and registration as a viewer. A database operator then follows the one-time transaction in the [initial administrator database procedure](docs/operations/009-首管理员数据库指定操作.md) to promote that registered user. Startup does not read an administrator email or password and requires no separate user-bootstrap command.

Verify the runtime:

```bash
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
```

- Swagger UI: <http://127.0.0.1:8080/docs>
- OpenAPI: <http://127.0.0.1:8080/openapi.json>
- Prometheus: <http://127.0.0.1:8080/metrics>

For production, set `HOTKEY_ENV=production` to load `.env.prod` as an override. Process environment variables always take precedence. API documentation routes are disabled in production.

### Run and debug with GoLand

Open this repository in GoLand and select the shared `HotKey 后端一键启动` run configuration:

- Use Run to start the backend or Debug to attach breakpoints.
- The entry point is fixed to `cmd/hotkey` and the working directory is the repository root, so the application loads the current `.env` automatically.
- The shared configuration lives in [`.run/HotKey_Server.run.xml`](.run/HotKey_Server.run.xml) and does not depend on a developer's `.idea/workspace.xml`.

GoLand uses the SDK and dependencies declared by `go.mod`. [`.editorconfig`](.editorconfig) keeps indentation, line endings, and trailing-whitespace behavior consistent across IDEs.

## Web application

Use [hotkey-web](https://github.com/StephenQiu30/hotkey-web) for the complete browser workspace, including events, monitors, sources, evidence, reports, and notification settings. The backend and Web application start and deploy separately, without `.sh` launchers.

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
docs/                      # Design, PRD, Plan, Acceptance, and Operations documents
tools/                     # Build-time tool dependencies
```

Business code stays under `internal/` so it cannot be imported outside this module. There is no `pkg/` directory because the repository currently exposes no reusable public Go library. The `all`, `api`, and `worker` values remain runtime roles of the single `cmd/hotkey` entry point.

`internal/shared` does not depend on an ORM, database driver, or `internal/platform`. GORM CRUD and PostgreSQL error mapping live in `internal/platform/database/repository`, while module infrastructure depends inward on the shared contracts.

Test sources and test-only fixtures live in package-mirrored paths below `test/_suite`. During execution, `test/runner` temporarily materializes `_test.go` files and package-local `testdata`, then removes those links, keeping test-only assets out of the `internal/` production tree.

This layout is an evidence-based choice rather than a copy of a purported standard template:

- [golang-standards/project-layout](https://github.com/golang-standards/project-layout) explicitly says it is not an official Go standard and recommends keeping only what a project needs.
- [ardanlabs/service](https://github.com/ardanlabs/service) structures a production starter around application, business, and foundation layers without requiring the same names used here.
- [ThreeDotsLabs/wild-workouts-go-ddd-example](https://github.com/ThreeDotsLabs/wild-workouts-go-ddd-example/tree/master/internal) organizes application, domain, ports, and adapters within business capabilities, which is the closest match to HotKey's module boundaries.
- [Grafana](https://github.com/grafana/grafana/tree/main/pkg/services), [Gitea](https://github.com/go-gitea/gitea/tree/main/services), and [Prometheus](https://github.com/prometheus/prometheus) use different service layouts, demonstrating that large Go projects do not converge on one directory tree.

The shared GoLand configuration follows the [official JetBrains Run/Debug configuration model](https://www.jetbrains.com/help/go/run-debug-configuration.html): it uses the `DIRECTORY` run kind with `$PROJECT_DIR$/cmd/hotkey` as the run directory and the repository root as the working directory, without depending on a developer-specific project module name.

```bash
make lint
make test
make build
make validate
make ci
```

The complete CI suite requires a disposable PostgreSQL database and a dedicated Redis test database. GitHub Actions checks OpenAPI drift, database behavior, tests, builds, schemas, and architectural boundaries.

## Project status

HotKey is under active development. The core end-to-end workflow is implemented, but APIs and deployment details may still change before 1.0. It is best suited for technical previews, self-hosted evaluation, and collaborative development rather than unattended critical production workloads.

Long-lived acceptance evidence is available in [`docs/acceptance/archive/`](docs/acceptance/archive/README.md). Roadmap and feature proposals are discussed publicly in [GitHub Issues](https://github.com/StephenQiu30/hotkey-server/issues). External dependencies, backups, and restores must still be exercised in each deployment environment using the [operations guides](docs/operations/README.md).

## Documentation

- [Architecture and design](docs/design/README.md)
- [Product requirements](docs/prd/README.md)
- [Implementation plans](docs/plans/README.md)
- [Acceptance evidence](docs/acceptance/README.md)
- [Operations guides](docs/operations/README.md)
- [OpenAPI JSON](docs/openapi/swagger.json)

## Contributing

Bug reports, use cases, connector proposals, documentation, and code contributions are welcome. Read the [contribution guide](CONTRIBUTING.md), [code of conduct](CODE_OF_CONDUCT.md), and [security policy](SECURITY.md) before getting started.

Please open an Issue before large changes. New connectors must document an official or authorized access path.

## License

HotKey Server is open source under the [MIT License](LICENSE).
