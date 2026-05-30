---
title: "HotKey Server Symphony Reboot Design"
date: "2026-05-31"
status: "draft-for-user-review"
scope: "hotkey-server"
owner: "StephenQiu30"
---

# HotKey Server Symphony Reboot Design

## 1. Decision

HotKey will restart from the `hotkey-server` repository first. The current goal is not to continue extending the old platform-shaped implementation. The goal is to cleanly rebuild a production-oriented Go backend for a personal creator AI hotspot monitoring product.

The server reboot must be orchestrated through the user's Symphony workflow. Symphony is treated as a fixed external workflow specification, not as a workflow to redesign inside HotKey. HotKey only adds the required workflow entrypoint and task prompts so Symphony can drive Linear issues through isolated workspaces.

The current phase is server-only. `hotkey-web` and `hotkey-miniapp` are not part of this cleanup or rebuild phase except as future OpenAPI consumers.

## 2. Product Scope

The rebooted server supports a personal creator MVP:

- Email registration and login are the primary account path.
- WeChat login is optional and requires explicit WeChat configuration.
- Users manage keywords and preferences.
- Users enable system sources and add RSS or public links.
- The server collects public content, normalizes it, deduplicates it, computes similarity, clusters related content, scores hotspots, and produces daily summaries.
- AI is used for readable summaries and daily report text. AI does not decide the ranking by itself.

The MVP excludes:

- Organization tenants.
- Complex RBAC.
- Billing.
- Enterprise admin consoles.
- Cross-language event graphs.
- Propagation arbitration.
- Complex worker pool design.
- Unauthorized scraping, private content collection, simulated login collection, or bypassing platform restrictions.

## 3. Symphony And Linear Integration

The first implementation task is to add a `WORKFLOW.md` that follows `StephenQiu30/symphony` exactly.

Rules:

- `WORKFLOW.md` uses the Symphony format: optional YAML front matter plus Markdown prompt body.
- Supported configuration belongs to Symphony's defined sections: `tracker`, `polling`, `workspace`, `hooks`, `agent`, and `codex`.
- The tracker is Linear.
- Linear issues are the task source of truth.
- Each Linear issue runs in an isolated Symphony workspace.
- Symphony handles polling, workspace creation, Codex app-server execution, retry behavior, and state coordination.
- HotKey does not add custom workflow semantics outside the Symphony spec.

HotKey task prompts inside `WORKFLOW.md` must tell the agent to:

- Work only on `hotkey-server` unless the Linear issue explicitly says otherwise.
- Read repository guidance before editing.
- Keep cleanup tasks separate from new feature tasks.
- Preserve unrelated user changes.
- Use standard Go project structure.
- Update tests and OpenAPI for API changes.
- Report verification commands in the Linear issue or pull request.

## 4. GitHub And Linear Governance

Old GitHub issues belong to the previous direction and should be closed as part of a dedicated cleanup issue. The close message should state that HotKey is restarting as a personal creator MVP server and that new work will be tracked in Linear through Symphony.

New work is created as Linear issues. The first Linear issue set should be:

1. Add Symphony `WORKFLOW.md` for `hotkey-server`.
2. Close legacy GitHub issues with a reboot notice.
3. Clean legacy server docs.
4. Clean legacy server business implementation.
5. Recreate the standard Go server skeleton.
6. Add base config, logger, errors, health check, and test baseline.
7. Add database migrations and core schema.
8. Add email-first authentication.
9. Add optional WeChat login configuration.
10. Add keyword management.
11. Add source management and compliant collection.
12. Add content normalization and deduplication.
13. Add embedding and hotspot clustering.
14. Add hotspot scoring and detail APIs.
15. Add AI summary and daily reports.
16. Add OpenAPI contract validation and deployment readiness.

## 5. Server Cleanup Boundary

Cleanup is a first-class phase, not incidental refactoring.

Keep:

- Git history.
- `AGENTS.md`, `CONTRIBUTING.md`, and `README.md`, then rewrite them for the new server direction.
- `go.mod` and `go.sum`, then prune dependencies as needed.
- Basic CI and repository governance files if present.
- The `docs/` directory structure, then replace old content with the new server plan.

Delete or archive through dedicated issues:

- Old platform PRDs, plans, and acceptance docs.
- Old `internal/httpapi/router.go` style large route implementation.
- Old OpenAPI generation coupled to deleted endpoints.
- Old in-memory repositories as the primary implementation path.
- Old platform modules such as tenant, RBAC, billing, event graph, propagation, realtime, and complex work queue.
- Old tests that only validate deleted platform scope.
- Old `db/schema.sql` as the schema source of truth.

The current worktree already contains uncommitted changes. Cleanup implementation must first decide whether to preserve, branch, or discard those changes. This design document does not delete them.

## 6. Standard Go Project Structure

The rebuilt server should use this structure:

```text
hotkey-server/
  cmd/
    hotkey-api/
      main.go
    hotkey-worker/
      main.go
  internal/
    app/
      api.go
      worker.go
    config/
      config.go
    platform/
      database/
      logger/
      clock/
      crypto/
      mailer/
      ai/
      wechat/
    domain/
      user/
      auth/
      keyword/
      source/
      content/
      hotspot/
      report/
    service/
      auth/
      keyword/
      source/
      ingest/
      hotspot/
      report/
    repository/
      postgres/
    transport/
      http/
        router.go
        middleware/
        handlers/
        dto/
      openapi/
    jobs/
      collector/
      embedding/
      clustering/
      scoring/
      reporting/
  migrations/
  db/
    schema.sql
  docs/
  tests/
  WORKFLOW.md
  go.mod
  Makefile
  README.md
```

Layer rules:

- `domain/` contains entities, value objects, and business rules. It does not depend on Gin, SQL, or external SDKs.
- `service/` coordinates use cases.
- `repository/postgres/` handles PostgreSQL persistence.
- `transport/http/` handles HTTP DTOs, request validation, response mapping, and routing.
- `platform/` wraps external systems such as database, logger, mailer, AI, and WeChat.
- `jobs/` contains background task entrypoints.
- `migrations/` is the database change source of truth.
- `db/schema.sql` is a generated or maintained snapshot, not the primary migration mechanism.

## 7. Development Flow

Every backend task follows the same flow:

1. Linear issue defines the task.
2. Symphony creates an isolated workspace.
3. The agent reads `WORKFLOW.md`, `AGENTS.md`, and the issue.
4. The agent writes or updates PRD and acceptance criteria when the issue is a planning task.
5. The agent writes tests before implementation for feature work.
6. The agent implements only the issue scope.
7. API changes update OpenAPI.
8. Database changes add migrations.
9. Verification includes `go test ./...` and any issue-specific commands.
10. Pull request and Linear updates include test evidence and remaining risks.

Commit messages remain Chinese and should use the existing categories: `test:`, `impl:`, `refactor:`, and `chore:`.

## 8. Core Data Model

The first schema should support the MVP without platform bloat:

- `users`: email, password hash, profile, status, login timestamps.
- `user_identities`: optional WeChat identity and future providers.
- `refresh_tokens` or `sessions`: token lifecycle and revocation.
- `keywords`: user keywords, weights, language, region, and enabled state.
- `sources`: system sources and user RSS or public link sources.
- `source_subscriptions`: user enablement state for sources.
- `source_items`: collected normalized content, URL, hash, source, publish time, and metadata.
- `item_embeddings`: embedding vectors and generation state.
- `hotspot_clusters`: related content groups.
- `hotspot_items`: source item membership, matched keywords, similarity, and primary item marker.
- `hotspot_scores`: relevance, heat, trust, freshness, total score, and explanation JSON.
- `ai_summaries`: AI summary outputs with model, prompt version, input references, and failure state.
- `daily_reports`: user daily report snapshots.
- `collection_runs`: source collection execution history.
- `job_runs`: task execution history for collection, embedding, clustering, scoring, and reporting.

## 9. Authentication Design

Email login is the primary path:

- Email registration.
- Email login.
- Password hashing.
- Password reset capability.
- Refresh token or session revocation.
- User profile endpoint.

WeChat login is optional:

- Disabled unless WeChat config is present.
- Requires AppID and AppSecret configuration.
- Uses the configured WeChat platform flow only.
- Does not block server startup or email login when disabled.
- Can later bind to an email account.

## 10. API Contract

The server remains the OpenAPI source of truth. Web and miniapp clients must generate from the server contract later.

Initial API groups:

- `GET /healthz`
- `POST /api/v1/auth/register`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `POST /api/v1/auth/wechat/login`
- `GET /api/v1/me`
- `GET /api/v1/keywords`
- `POST /api/v1/keywords`
- `PATCH /api/v1/keywords/{id}`
- `DELETE /api/v1/keywords/{id}`
- `GET /api/v1/sources`
- `POST /api/v1/sources`
- `PATCH /api/v1/sources/{id}`
- `GET /api/v1/hotspots`
- `GET /api/v1/hotspots/{id}`
- `GET /api/v1/reports/daily`
- `POST /api/v1/reports/daily/generate`

## 11. Error Handling And Observability

The server should standardize:

- Structured error response with code, message, and request id.
- Request id middleware.
- Structured logs.
- Health check endpoint.
- Startup configuration validation.
- External dependency status for database, AI, mail, and WeChat.
- Safe degradation when AI or WeChat is disabled.

## 12. Acceptance Criteria

This design is accepted when:

- The user approves this document as the server reboot direction.
- The next step invokes the writing-plans skill.
- The implementation plan starts with Symphony `WORKFLOW.md` and Linear orchestration.
- Cleanup and new implementation tasks are split.
- No implementation file is changed before the approved plan starts.

## 13. Sources

- Symphony SPEC: https://raw.githubusercontent.com/StephenQiu30/symphony/main/SPEC.md
- Symphony example workflow: https://raw.githubusercontent.com/StephenQiu30/symphony/main/elixir/WORKFLOW.md
