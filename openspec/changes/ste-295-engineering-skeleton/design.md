---
change: ste-295-engineering-skeleton
type: design
---

## Architecture

```
cmd/hotkey/main.go          Cobra root command (< 50 lines)
cmd/hotkey/api.go           api subcommand → Wire + Fx
cmd/hotkey/worker.go        worker subcommand → Wire + Fx
internal/app/wire.go        Wire provider sets (build-tag: wireinject)
internal/app/wire_gen.go    Wire generated injectors
internal/app/api.go         Fx app for API server
internal/app/worker.go      Fx app for worker
internal/app/stubs.go       Smoke test / worker stubs
internal/config/config.go   Viper-based config loading
internal/platform/config/   Fx config module
```

## DI Flow

```
Cobra command
  → app.InitializeAPI() [Wire generated]
    → config.Load() [Viper]
  → app.NewAPIApp(cfg) [Fx]
    → fx.Supply(cfg)
    → fx.Invoke(startAPIServer)
      → database.Open / handler wiring / http.Server
      → fx.Lifecycle hooks (OnStart/OnStop)
```

## Decisions

- Wire handles compile-time DI for config; Fx handles runtime lifecycle.
- Smoke stubs remain in `internal/app/stubs.go` for Wire accessibility.
- Old `cmd/api/main.go` is preserved for backward compatibility with existing tests.
- HTTP routing is unchanged (server/router.go untouched).
