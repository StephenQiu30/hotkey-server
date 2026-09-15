# HotKey

A personal, noncommercial learning project for social-media monitoring. The only server stack is Python, FastAPI, SQLAlchemy 2, PostgreSQL, Alembic, Celery and RabbitMQ. The workspace uses React, TypeScript and Vite.

Implemented: single-owner initialization, revocable cookie sessions, CSRF protection, versioned monitor drafts, and idempotent diagnostic jobs through a transactional Outbox and a real prefork worker.

Bluesky query preview and bounded CLI source probes are implemented. Public thread reads preserve reply relationships and partial results; the September 8, 2026 search probe returned HTTP 403 in the tested environment. **Persistent collection jobs, domestic sources, event grouping, analysis and reports are not implemented yet.** Saving a draft does not start collection. Diagnostic jobs do not produce social data.

The September 15 replanning defines **monitors → discovery inbox → comment tracking → event records → analysis and knowledge retrieval**. See the [007 requirements](docs/prd/007-热点事件与评论知识库.md), [data and technical design](docs/design/007-热点事件与评论知识库设计.md), and [delivery plan](docs/plans/007-热点事件与评论知识库计划.md). The S00 engineering guardrails are in progress; product collection and knowledge capabilities remain planned. The 006 documents retain the existing implementation and verification record.

From the repository root, with Docker Compose v2.24.4+:

```sh
docker compose up -d --build
docker compose exec backend python -m cli owner-init learner
```

Enter a password of at least 12 characters interactively and visit http://localhost:8010. No default owner exists. Ports bind to loopback only. Use the production override behind HTTPS for a production-like experiment.

See the [Chinese README](README.md), [design and source research](docs/design/006-社交媒体关键词监控与评论分析设计.md), [implementation plan](docs/plans/006-社交媒体关键词监控与评论分析计划.md), and [operations](docs/operations/006-Python运行与验证.md). FastAPI generates the runtime OpenAPI document and Swagger UI. Its reproducible snapshot is `docs/openapi/openapi.json`; `@umijs/openapi` generates all frontend endpoint functions and types under `frontend/src/api/`, while Axios is confined to `frontend/src/request.ts`.

The previous implementation and contracts exist only in Git history. No legacy endpoints, accounts or databases are silently reused. Existing persistent volumes are preserved.

Backend application code lives in `backend/src/`, with separate `worker/` and `cli/` directories. Tests and verification scripts stay in `backend/tests/` and `backend/scripts/`. See the [backend layout and commands](backend/README.md).
