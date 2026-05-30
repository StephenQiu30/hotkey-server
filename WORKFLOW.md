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
