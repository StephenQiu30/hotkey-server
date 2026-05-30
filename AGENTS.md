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
