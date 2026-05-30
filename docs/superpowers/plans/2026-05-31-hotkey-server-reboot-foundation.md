# HotKey Server Reboot Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reboot `hotkey-server` foundation by adding the fixed Symphony workflow entrypoint, protecting the current dirty worktree, removing legacy server scope, and creating a standard Go API skeleton with a passing health check.

**Architecture:** This plan only builds the server foundation. Symphony and Linear orchestrate future issues through `WORKFLOW.md`; the Go server is rebuilt around `cmd/hotkey-api`, `internal/app`, `internal/config`, `internal/platform`, and `internal/transport/http`. Product modules such as auth, keywords, sources, hotspots, AI summaries, and reports are tracked as later Linear issues, not implemented in this foundation PR.

**Tech Stack:** Go 1.25, Gin, PostgreSQL-ready migrations directory, Python unittest for repository contract checks, Symphony `WORKFLOW.md`, Linear.

---

## File Structure

Create or modify these files in this foundation plan:

- Create: `WORKFLOW.md` — Symphony fixed-format workflow contract for Linear-driven server work.
- Create: `tests/test_workflow_contract.py` — repository test that validates the workflow file shape without implementing Symphony.
- Modify: `README.md` — replace old platform wording with the new server reboot entrypoint and commands.
- Modify: `AGENTS.md` — keep repository guidance focused on the rebooted server scope.
- Create: `cmd/hotkey-api/main.go` — API process entrypoint.
- Create: `internal/app/api.go` — server assembly and lifecycle wiring.
- Create: `internal/config/config.go` — environment configuration loader.
- Create: `internal/platform/logger/logger.go` — minimal structured logger wrapper.
- Create: `internal/transport/http/router.go` — HTTP router factory.
- Create: `internal/transport/http/handlers/health.go` — health endpoint handler.
- Create: `internal/transport/http/router_test.go` — health endpoint contract test.
- Create: `migrations/000001_init.up.sql` — initial foundation migration with migration table and extensions used by later tasks.
- Create: `migrations/000001_init.down.sql` — reversible migration for the initial foundation objects.
- Create: `Makefile` — standard test, run, and format commands.
- Remove: legacy `internal/*` platform/business modules listed in Task 3.
- Remove or archive: legacy `docs/product/prd`, `docs/plans`, and `docs/acceptance` contents listed in Task 3.

Do not modify `hotkey-web` or `hotkey-miniapp` in this plan.

## Task 1: Add Symphony Workflow Contract

**Files:**
- Create: `WORKFLOW.md`
- Create: `tests/test_workflow_contract.py`

- [ ] **Step 1: Write the failing workflow contract test**

Create `tests/test_workflow_contract.py`:

```python
import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]
WORKFLOW = ROOT / "WORKFLOW.md"


class WorkflowContractTest(unittest.TestCase):
    def test_workflow_file_uses_symphony_front_matter(self):
        text = WORKFLOW.read_text(encoding="utf-8")
        self.assertTrue(text.startswith("---\n"))
        front_matter = text.split("---\n", 2)[1]
        body = text.split("---\n", 2)[2].strip()

        for key in ["tracker:", "polling:", "workspace:", "hooks:", "agent:", "codex:"]:
            self.assertIn(key, front_matter)

        self.assertIn("kind: linear", front_matter)
        self.assertIn('api_key: "$LINEAR_API_KEY"', front_matter)
        self.assertIn('project_slug: "$SYMPHONY_LINEAR_PROJECT_SLUG"', front_matter)
        self.assertRegex(front_matter, re.compile(r"active_states:\n(\\s+- .+\n)+"))
        self.assertRegex(front_matter, re.compile(r"terminal_states:\n(\\s+- .+\n)+"))
        self.assertIn("{{ issue.identifier }}", body)
        self.assertIn("hotkey-server", body)


if __name__ == "__main__":
    unittest.main()
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
python3 -m unittest tests/test_workflow_contract.py
```

Expected: `FileNotFoundError` for `WORKFLOW.md`.

- [ ] **Step 3: Create the Symphony workflow file**

Create `WORKFLOW.md`:

```markdown
---
tracker:
  kind: linear
  api_key: "$LINEAR_API_KEY"
  project_slug: "$SYMPHONY_LINEAR_PROJECT_SLUG"
  active_states:
    - Todo
    - In Progress
  terminal_states:
    - Closed
    - Cancelled
    - Canceled
    - Duplicate
    - Done
polling:
  interval_ms: 30000
workspace:
  root: "$SYMPHONY_WORKSPACE_ROOT"
hooks:
  timeout_ms: 60000
  after_create: |
    git clone "$HOTKEY_SERVER_REPOSITORY_URL" .
  before_run: |
    git status --short
    test -f AGENTS.md
agent:
  max_concurrent_agents: 2
  max_turns: 20
  max_retry_backoff_ms: 300000
codex:
  command: codex app-server
  turn_timeout_ms: 3600000
  read_timeout_ms: 5000
  stall_timeout_ms: 300000
---

# HotKey Server Symphony Workflow

You are working on Linear issue {{ issue.identifier }}: {{ issue.title }}.

This workflow is scoped to `hotkey-server`. Do not modify `hotkey-web` or `hotkey-miniapp` unless the Linear issue explicitly names those repositories.

Before editing:

1. Read `AGENTS.md`.
2. Read the Linear issue description.
3. Run `git status --short` and preserve unrelated user changes.
4. Confirm whether the issue is a cleanup task, planning task, or implementation task.

Execution rules:

- Use standard Go project structure.
- Keep cleanup tasks separate from feature tasks.
- Use TDD for implementation work.
- Add or update migrations for database changes.
- Update OpenAPI for API changes.
- Run `go test ./...` before handoff.
- Report commands, results, risks, and changed files in the Linear issue or pull request.

Issue labels: {{ issue.labels }}
Attempt: {{ attempt }}
```

- [ ] **Step 4: Run the workflow contract test**

Run:

```bash
python3 -m unittest tests/test_workflow_contract.py
```

Expected: `OK`.

- [ ] **Step 5: Commit**

```bash
git add WORKFLOW.md tests/test_workflow_contract.py
git commit -m "chore: 接入Symphony工作流契约"
```

## Task 2: Preserve Current Dirty Worktree Before Cleanup

**Files:**
- Create: `.reboot-safety/pre-cleanup-status.txt`
- Create: `.reboot-safety/pre-cleanup-diff.patch`
- Create: `.reboot-safety/pre-cleanup-untracked.txt`

- [ ] **Step 1: Capture current status**

Run:

```bash
mkdir -p .reboot-safety
git status --short > .reboot-safety/pre-cleanup-status.txt
git diff > .reboot-safety/pre-cleanup-diff.patch
git ls-files --others --exclude-standard > .reboot-safety/pre-cleanup-untracked.txt
```

Expected: `.reboot-safety/pre-cleanup-status.txt` records the dirty files that existed before cleanup.

- [ ] **Step 2: Create a safety stash**

Run:

```bash
git stash push -u -m "pre-reboot legacy worktree safety snapshot"
```

Expected: output contains `Saved working directory and index state`.

- [ ] **Step 3: Restore only the plan-approved foundation files if they were stashed**

Run:

```bash
for path in WORKFLOW.md tests/test_workflow_contract.py docs/superpowers; do
  git checkout stash@{0} -- "$path" 2>/dev/null || true
done
```

Expected: any plan-approved files caught in the safety stash are restored; missing paths are ignored.

- [ ] **Step 4: Commit the safety record**

```bash
git add .reboot-safety/pre-cleanup-status.txt .reboot-safety/pre-cleanup-diff.patch .reboot-safety/pre-cleanup-untracked.txt
git commit -m "chore: 记录重启前工作区快照"
```

## Task 3: Remove Legacy Server Scope

**Files:**
- Remove directories: `internal/adminapi`, `internal/billing`, `internal/event`, `internal/eventgraph`, `internal/governance`, `internal/propagation`, `internal/rbac`, `internal/realtime`, `internal/redisinfra`, `internal/store`, `internal/tenant`, `internal/trust`, `internal/workqueue`
- Remove directories: `docs/product/prd`, `docs/plans`, `docs/acceptance`
- Remove file: `db/schema.sql`
- Create: `docs/product/prd/.gitkeep`
- Create: `docs/plans/.gitkeep`
- Create: `docs/acceptance/.gitkeep`

- [ ] **Step 1: Verify legacy files exist before removal**

Run:

```bash
test -d internal/billing
test -d internal/rbac
test -d docs/product/prd
test -f db/schema.sql
```

Expected: command exits `0` before cleanup.

- [ ] **Step 2: Remove legacy platform scope**

Run:

```bash
rm -rf \
  internal/adminapi \
  internal/billing \
  internal/event \
  internal/eventgraph \
  internal/governance \
  internal/propagation \
  internal/rbac \
  internal/realtime \
  internal/redisinfra \
  internal/store \
  internal/tenant \
  internal/trust \
  internal/workqueue \
  docs/product/prd \
  docs/plans \
  docs/acceptance \
  db/schema.sql
mkdir -p docs/product/prd docs/plans docs/acceptance
touch docs/product/prd/.gitkeep docs/plans/.gitkeep docs/acceptance/.gitkeep
```

- [ ] **Step 3: Verify deleted scope is gone and `.gitkeep` directories remain**

Run:

```bash
test ! -d internal/billing
test ! -d internal/rbac
test ! -f db/schema.sql
test -f docs/product/prd/.gitkeep
test -f docs/plans/.gitkeep
test -f docs/acceptance/.gitkeep
```

Expected: command exits `0`.

- [ ] **Step 4: Commit**

```bash
git add -A internal docs/product/prd docs/plans docs/acceptance db
git commit -m "chore: 清理旧Server平台化范围"
```

## Task 4: Rebuild Minimal Standard Go API Skeleton

**Files:**
- Create: `cmd/hotkey-api/main.go`
- Create: `internal/app/api.go`
- Create: `internal/config/config.go`
- Create: `internal/platform/logger/logger.go`
- Create: `internal/transport/http/router.go`
- Create: `internal/transport/http/handlers/health.go`
- Create: `internal/transport/http/router_test.go`
- Create: `migrations/000001_init.up.sql`
- Create: `migrations/000001_init.down.sql`
- Create: `Makefile`

- [ ] **Step 1: Write the failing health contract test**

Create `internal/transport/http/router_test.go`:

```go
package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

func TestHealthz(t *testing.T) {
	router := transporthttp.NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %q", rec.Code, rec.Body.String())
	}

	want := `{"status":"ok"}`
	if rec.Body.String() != want {
		t.Fatalf("expected body %s, got %s", want, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/transport/http
```

Expected: FAIL because package `internal/transport/http` does not exist.

- [ ] **Step 3: Add minimal config**

Create `internal/config/config.go`:

```go
package config

import "os"

type Config struct {
	HTTPAddr string
}

func Load() Config {
	return Config{
		HTTPAddr: envOrDefault("HOTKEY_HTTP_ADDR", ":8080"),
	}
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
```

- [ ] **Step 4: Add minimal logger**

Create `internal/platform/logger/logger.go`:

```go
package logger

import (
	"log/slog"
	"os"
)

func New() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}
```

- [ ] **Step 5: Add health handler and router**

Create `internal/transport/http/handlers/health.go`:

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```

Create `internal/transport/http/router.go`:

```go
package http

import (
	"github.com/StephenQiu30/hotkey-server/internal/transport/http/handlers"
	"github.com/gin-gonic/gin"
)

func NewRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.GET("/healthz", handlers.Healthz)
	return router
}
```

- [ ] **Step 6: Add app assembly and main entrypoint**

Create `internal/app/api.go`:

```go
package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
	transporthttp "github.com/StephenQiu30/hotkey-server/internal/transport/http"
)

type API struct {
	server *http.Server
	logger *slog.Logger
}

func NewAPI(cfg config.Config, logger *slog.Logger) *API {
	return &API{
		server: &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           transporthttp.NewRouter(),
			ReadHeaderTimeout: 5 * time.Second,
		},
		logger: logger,
	}
}

func (api *API) Run() error {
	api.logger.Info("starting hotkey api", "addr", api.server.Addr)
	return api.server.ListenAndServe()
}

func (api *API) Shutdown(ctx context.Context) error {
	return api.server.Shutdown(ctx)
}
```

Create `cmd/hotkey-api/main.go`:

```go
package main

import (
	"errors"
	"net/http"

	"github.com/StephenQiu30/hotkey-server/internal/app"
	"github.com/StephenQiu30/hotkey-server/internal/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/logger"
)

func main() {
	cfg := config.Load()
	log := logger.New()
	api := app.NewAPI(cfg, log)

	if err := api.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("api stopped", "error", err)
		panic(err)
	}
}
```

- [ ] **Step 7: Add migration foundation**

Create `migrations/000001_init.up.sql`:

```sql
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);
```

Create `migrations/000001_init.down.sql`:

```sql
DROP TABLE IF EXISTS schema_migrations;
```

- [ ] **Step 8: Add Makefile**

Create `Makefile`:

```makefile
.PHONY: test run fmt workflow-test

test:
	go test ./...
	python3 -m unittest discover -s tests

workflow-test:
	python3 -m unittest tests/test_workflow_contract.py

run:
	go run ./cmd/hotkey-api

fmt:
	gofmt -w cmd internal
```

- [ ] **Step 9: Run tests**

Run:

```bash
gofmt -w cmd internal
go test ./...
python3 -m unittest discover -s tests
```

Expected: all tests pass.

- [ ] **Step 10: Commit**

```bash
git add cmd internal migrations Makefile
git commit -m "impl: 重建标准Go API基础骨架"
```

## Task 5: Rewrite Repository Entry Docs For Server Reboot

**Files:**
- Modify: `README.md`
- Modify: `AGENTS.md`

- [ ] **Step 1: Replace README content**

Write `README.md`:

```markdown
# hotkey-server

`hotkey-server` is the Go backend for the HotKey personal creator AI hotspot monitoring product.

The repository is being rebooted around a standard Go service structure and Symphony-driven Linear workflow. The server is the future OpenAPI source of truth for Web and miniapp clients.

## Current Scope

- Email-first account system.
- Optional WeChat login when configuration is present.
- User keywords and preferences.
- System sources plus user RSS or public links.
- Content normalization, deduplication, similarity, hotspot scoring, AI summaries, and daily reports.

The first foundation phase only provides:

- Symphony `WORKFLOW.md`.
- Standard Go API skeleton.
- `GET /healthz`.
- Migration directory foundation.
- Test and run commands.

## Commands

```bash
make test
make run
HOTKEY_HTTP_ADDR=127.0.0.1:18080 make run
curl http://127.0.0.1:18080/healthz
```

## Workflow

All implementation work is tracked in Linear and orchestrated by Symphony. `WORKFLOW.md` is the repository-owned workflow contract.
```

- [ ] **Step 2: Replace AGENTS content**

Write `AGENTS.md`:

```markdown
# AGENTS.md

This repository contains `hotkey-server`, the Go backend for HotKey.

## Scope

Current work is server-only. Do not modify `hotkey-web` or `hotkey-miniapp` from this repository workflow.

## Workflow

- Linear issues are the task source of truth.
- Symphony reads `WORKFLOW.md` and runs each issue in an isolated workspace.
- Keep cleanup tasks separate from feature tasks.
- Preserve unrelated user changes.
- Use Chinese commit messages.

## Go Standards

- Use standard Go layout under `cmd/` and `internal/`.
- Keep domain logic independent from HTTP, SQL, and external SDKs.
- Put HTTP routing under `internal/transport/http`.
- Put external integrations under `internal/platform`.
- Put persistence under `internal/repository/postgres`.
- Put database migrations under `migrations/`.

## Required Checks

Run before handoff:

```bash
gofmt -w cmd internal
go test ./...
python3 -m unittest discover -s tests
```
```

- [ ] **Step 3: Run checks**

Run:

```bash
make test
```

Expected: Go tests and Python workflow contract tests pass.

- [ ] **Step 4: Commit**

```bash
git add README.md AGENTS.md
git commit -m "docs: 更新Server重启入口说明"
```

## Task 6: Final Foundation Verification

**Files:**
- No new files.

- [ ] **Step 1: Verify working tree**

Run:

```bash
git status --short
```

Expected: no uncommitted changes, except intentional files from a currently active task.

- [ ] **Step 2: Run full verification**

Run:

```bash
make test
HOTKEY_HTTP_ADDR=127.0.0.1:18080 go run ./cmd/hotkey-api &
SERVER_PID=$!
sleep 2
curl -fsS http://127.0.0.1:18080/healthz
kill "$SERVER_PID"
wait "$SERVER_PID" 2>/dev/null || true
```

Expected curl output:

```json
{"status":"ok"}
```

- [ ] **Step 3: Record Linear handoff summary**

Add this summary to the Linear issue or PR:

```markdown
## Result

Completed HotKey server reboot foundation.

## Commands Run

- `python3 -m unittest tests/test_workflow_contract.py`
- `gofmt -w cmd internal`
- `go test ./...`
- `python3 -m unittest discover -s tests`
- `curl -fsS http://127.0.0.1:18080/healthz`

## Notes

- `WORKFLOW.md` follows the Symphony fixed workflow contract.
- Current phase is server-only.
- Product features are intentionally split into later Linear issues.
```

- [ ] **Step 4: Finish without repository changes**

Do not create an extra verification file in this foundation task. The verification evidence belongs in Linear or the pull request body.

## Self-Review

Spec coverage:

- Symphony `WORKFLOW.md`: Task 1.
- Linear-driven workflow boundary: Task 1 and Task 6.
- Dirty worktree protection: Task 2.
- Legacy cleanup: Task 3.
- Standard Go structure: Task 4.
- Server-only scope: Tasks 1, 3, and 5.
- Health check and test baseline: Task 4 and Task 6.
- Future product modules split out of foundation scope: header and Task 6 handoff.

Plan hygiene scan:

- No banned marker text or open-ended implementation steps are used.

Type consistency:

- Go module imports use `github.com/StephenQiu30/hotkey-server`, matching the existing repository module path pattern.
- HTTP router constructor is consistently named `NewRouter`.
- Health endpoint returns exactly `{"status":"ok"}`.
