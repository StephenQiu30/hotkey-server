<p align="center">
  <img src="src/app/icon.svg" width="72" alt="HotKey logo">
</p>

<h1 align="center">HotKey Web</h1>

<p align="center"><a href="README.md">简体中文</a> · <a href="README_EN.md">English</a></p>

<p align="center">
  <strong>An open-source AI event intelligence workspace for creators and researchers.</strong>
</p>

<p align="center">
  <a href="https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/StephenQiu30/hotkey-server/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <a href="https://nextjs.org/"><img alt="Next.js 16" src="https://img.shields.io/badge/Next.js-16-black?logo=next.js"></a>
  <a href="https://www.typescriptlang.org/"><img alt="TypeScript" src="https://img.shields.io/badge/TypeScript-5.9-3178C6?logo=typescript&logoColor=white"></a>
  <a href="../LICENSE"><img alt="MIT License" src="https://img.shields.io/badge/license-MIT-green.svg"></a>
</p>

HotKey turns public signals from RSS, Atom, Hacker News, and other compliant sources into verifiable events, editorial ideas, and daily or weekly reports. `hotkey-web` is the desktop workspace for managing monitors and sources, reading evidence, understanding events, publishing reports, and configuring delivery.

> If this direction is useful to you, consider starring the project, sharing a real-world use case, or contributing to the UI, visualizations, accessibility, and documentation.

## Repository projects

HotKey keeps its frontend and backend as two projects in one repository:

| Directory                                                      | Responsibility                                                                        |
| -------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| [`frontend/`](.)                                               | The user-facing desktop Web workspace                                                  |
| [`backend/`](../backend/README_EN.md)                          | Backend APIs, collection and AI jobs, data models, delivery, and the OpenAPI contract |

The projects can be developed and deployed independently. Run both to use the complete product.

## What you can do

- **Find accelerating events** through heat, trend, and recency instead of browsing isolated items.
- **Inspect the evidence** through event members, timelines, entities, claims, sources, and original Markdown documents.
- **Manage long-running monitors** with multilingual rules, source configuration, previews, and explicit publishing.
- **Turn signals into output** by building and publishing daily or weekly reports to Obsidian, email, and private RSS/Atom.
- **Keep control of your data** because the browser talks to your own Next.js and [hotkey-server](https://github.com/StephenQiu30/hotkey-server) deployment.
- **Operate from one workspace** with identity, content, favorites, sources, notifications, profile, and settings screens.

## Workflow

```text
Public sources → Monitors → Evidence → Events → AI analysis → Reports and knowledge delivery
                                ↑
                         HotKey Web workspace
```

The frontend does not hand-write backend DTOs. Request functions and types are generated from the `hotkey-server` OpenAPI document.

## Stack

| Area              | Technology                                          |
| ----------------- | --------------------------------------------------- |
| Application       | Next.js 16 App Router, React 19, TypeScript 5.9     |
| Styling           | Tailwind CSS 4, CSS variables, dark theme           |
| UI foundations    | Radix UI, Lucide Icons, composed local components   |
| Charts and motion | Recharts, GSAP                                      |
| Data and state    | Axios, Zustand, generated OpenAPI client            |
| Testing           | Vitest, Testing Library, Playwright / agent-browser |

## Quick start

### Requirements

- Node.js 22 (the CI version)
- npm
- A running [hotkey-server](https://github.com/StephenQiu30/hotkey-server), available at `http://127.0.0.1:8080` by default

### Local development

```bash
git clone https://github.com/StephenQiu30/hotkey-server.git
cd hotkey-server/frontend
npm ci
cp .env.example .env
npm run dev
```

Open <http://localhost:3000>.

No environment value is required for the default local backend. Same-origin `/api` and `/healthz` requests are sent through the Next.js server to:

```dotenv
HOTKEY_API_ORIGIN=http://127.0.0.1:8080
```

This variable is server-only and is not exposed to the browser as `NEXT_PUBLIC_*`. See [`.env.example`](.env.example) for the complete configuration.

In WebStorm, you can run the `dev` script from `package.json` directly. The Web application and backend start separately and require no `.sh` launcher. See the [`backend/` documentation](../backend/README_EN.md) for registration, email, and administrator configuration.

### Docker

Docker Compose is centralized in a root default configuration with a production override and starts the frontend, backend, PostgreSQL, Redis, and MinIO together. Start the default environment with:

```bash
cd ..
cp backend/.env.example backend/.env
cp .env.example .env
docker compose -f docker-compose.yml up --build -d
```

Create the root production configuration and fill the required credentials before the first production run:

```bash
cp .env.prod.example .env.prod
docker compose --env-file .env.prod -f docker-compose-prod.yml config
docker compose --env-file .env.prod -f docker-compose-prod.yml up --build -d
```

The shared baseline connects the frontend and backend through internal service names instead of `host.docker.internal`; overrides contain environment differences only. The frontend Dockerfile remains in `frontend/`, while root Compose owns environment, port, and service orchestration.

## Commands

```bash
npm run dev
npm run typecheck
npm run test:unit
npm run build
npm run openapi:generate
npm run openapi:check
```

Only regenerate the client when the backend OpenAPI contract changes. Generated files live under `src/services/hotkey/hotkey-server/` and should not be edited manually.

The integration uses the official `@umijs/openapi` `openapi2ts` CLI. Update and generate the backend contract at `../docs/openapi/swagger.json`, regenerate the client, review the generated diff, and run `openapi:check` to verify the published contract and generated output are in sync. `HOTKEY_OPENAPI_SCHEMA` may temporarily override the source. Application code should call generated functions instead of declaring endpoint paths, request DTOs, or response types by hand.

## Project structure

```text
src/app/                         # Routes and pages
src/components/                  # Product and UI components
src/layouts/                     # Workspace layouts
src/lib/                         # HTTP, auth session, and utilities
src/stores/                      # Zustand stores
src/services/hotkey/             # Generated OpenAPI client
test/                            # Centralized unit tests
../docs/                # Product, design, plan, acceptance, and operations docs
```

## Project status

HotKey Web is under active development. Authentication, the main intelligence workspace, monitors, sources, content evidence, reports, favorites, notifications, profile, and settings are connected to the real backend contract. Navigation, visual details, and selected workflows will continue to evolve before 1.0.

The current version is intended for local use, self-hosted evaluation, and collaborative development. Roadmap and feature proposals are discussed publicly in [GitHub Issues](https://github.com/StephenQiu30/hotkey-server/issues). When reporting UI problems, include the browser, viewport, reproduction steps, and screenshots when possible.

## Contributing

Contributions to accessibility, responsive behavior, visualizations, testing, performance, documentation, and new backend-backed workflows are welcome.

Read the [contribution guide](../CONTRIBUTING.md) and [security policy](../SECURITY.md). Please open an Issue before large changes and describe the user problem, interaction scope, and acceptance criteria.

## Repository projects

| Directory                                                      | Purpose                                                |
| -------------------------------------------------------------- | ------------------------------------------------------ |
| [`backend/`](../backend/README_EN.md)                          | Backend, jobs, data model, and OpenAPI source of truth |
| [`frontend/`](.)                                               | The desktop Web workspace                              |

## License

HotKey Web is open source under the repository-level [MIT License](../LICENSE). The `private: true` field in `package.json` only prevents accidental npm publication; it does not make the GitHub project proprietary.
