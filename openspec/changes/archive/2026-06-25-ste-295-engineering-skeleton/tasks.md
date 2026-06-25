---
change: ste-295-engineering-skeleton
type: tasks
---

## Tasks

- [x] 1. Add Cobra + Viper + Wire + Fx dependencies to go.mod
- [x] 2. Refactor `internal/config/config.go` to use Viper
- [x] 3. Create `internal/platform/config/module.go` Fx module
- [x] 4. Create `internal/app/stubs.go` with smoke/worker stubs
- [x] 5. Create `internal/app/api.go` Fx app for API
- [x] 6. Create `internal/app/worker.go` Fx app for Worker
- [x] 7. Create `internal/app/wire.go` Wire provider sets
- [x] 8. Create `internal/app/wire_gen.go` Wire generated injectors
- [x] 9. Create `cmd/hotkey/main.go` Cobra entry (< 50 lines)
- [x] 10. Create `cmd/hotkey/api.go` API subcommand
- [x] 11. Create `cmd/hotkey/worker.go` Worker subcommand
- [x] 12. Update `Dockerfile` build path to `./cmd/hotkey`
- [x] 13. Verify: `go build ./...` passes
- [x] 14. Verify: `go test` (non-integration) passes
- [x] 15. Verify: `go vet ./...` passes
- [x] 16. Verify: Cobra subcommands work (`--help`)
- [x] 17. Verify: main.go < 50 lines (23 lines)
