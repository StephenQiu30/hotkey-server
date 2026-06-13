---
change: ste-295-engineering-skeleton
title: Layer 1 工程骨架：Cobra + Viper + Fx + Wire
status: implemented
---

## Proposal

Replace the hand-written `os.Args` CLI dispatch and raw `os.Getenv` config loading with
Cobra (CLI), Viper (config), Wire (compile-time DI), and Fx (lifecycle management).

## Scope

- `cmd/hotkey/` — Cobra root + `api` / `worker` subcommands
- `internal/config/config.go` — Viper-based config loading
- `internal/platform/config/` — Fx config module
- `internal/app/` — Wire provider sets, Fx app constructors, smoke stubs
- `Dockerfile`, `Makefile` — updated build entry point

## Non-goals

- HTTP routing changes (deferred to Layer 2)
- Database/Redis/Queue wiring beyond config (deferred to Layer 3)
- Removing old `cmd/api/main.go` (kept for backward compatibility)

## Validation

```bash
make test && make validate && go build ./...
```
