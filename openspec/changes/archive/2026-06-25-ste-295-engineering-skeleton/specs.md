---
change: ste-295-engineering-skeleton
type: specs
---

## Normative Requirements

### CLI (ADR-003)

1. The binary MUST expose `api` and `worker` subcommands via Cobra.
2. `hotkey-server api --help` and `hotkey-server worker --help` MUST print usage.
3. Root command MUST be `hotkey-server`.

### Config (ADR-004)

4. `internal/config/config.go` MUST use Viper to load configuration.
5. Viper MUST bind `DATABASE_URL`, `JWT_SECRET`, `HTTP_ADDR`, `REDIS_ADDR` from environment variables.
6. Viper MUST attempt to read `.env` file (non-fatal if missing).
7. `Load()` MUST return error if `DATABASE_URL` is empty.
8. `Load()` MUST return error if `JWTSecret` is empty.
9. Default `HTTP_ADDR` MUST be `:8080`.

### DI (ADR-006)

10. Wire MUST generate `InitializeAPI()` and `InitializeWorker()` injector functions in `internal/app/wire_gen.go`.
11. Wire provider set MUST include `config.Load`.

### Lifecycle (ADR-005)

12. `NewAPIApp()` MUST return an `*fx.App` that starts the HTTP server.
13. `NewWorkerApp()` MUST return an `*fx.App` that runs background jobs.
14. Both apps MUST support graceful shutdown via Fx lifecycle hooks.

### Entry Point

15. `cmd/hotkey/main.go` MUST be under 50 lines.
16. The binary MUST build from `./cmd/hotkey`.
