# HotKey

A personal, noncommercial learning project for social-media monitoring. The only server stack is Python, FastAPI, SQLAlchemy 2, PostgreSQL, Alembic, Celery and RabbitMQ. The workspace uses React, TypeScript and Vite.

Implemented: single-owner initialization, revocable cookie sessions, CSRF protection, versioned monitor drafts, and idempotent diagnostic jobs through a transactional Outbox and a real prefork worker.

**Social keyword search, post/comment/reply collection, event grouping, analysis and reports are not implemented yet.** Saving a draft does not start collection. Diagnostic jobs do not produce social data.

From the repository root, with Docker Compose v2.24.4+:

```sh
docker compose up -d --build
docker compose exec backend python -m cli owner-init learner
```

Enter a password of at least 12 characters interactively and visit http://localhost:8010. No default owner exists. Ports bind to loopback only. Use the production override behind HTTPS for a production-like experiment.

See the [Chinese README](README.md), [design and source research](docs/design/006-社交媒体关键词监控与评论分析设计.md), [implementation plan](docs/plans/006-社交媒体关键词监控与评论分析计划.md), and [operations](docs/operations/006-Python运行与验证.md). The published API contract is `docs/openapi/openapi.json`; frontend types are generated from it.

The previous implementation and contracts exist only in Git history. No legacy endpoints, accounts or databases are silently reused. Existing persistent volumes are preserved.

Backend application code lives in `backend/src/`, with separate `worker/` and `cli/` directories. Tests and verification scripts stay in `backend/tests/` and `backend/scripts/`. See the [backend layout and commands](backend/README.md).
