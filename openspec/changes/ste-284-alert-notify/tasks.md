# Tasks: STE-284 提醒通知闭环

## Task 1: Wire notification handler into router

- [ ] Add `NotificationHandler http.Handler` to `server.Dependencies`
- [ ] Mount notification routes in `NewRouter` (with auth middleware)
- [ ] Add router test for notification routes requiring auth

**Files:** `internal/server/router.go`, `internal/server/router_test.go`

**Validation:** `go test ./internal/server/... -v`

## Task 2: Wire notification handler in main entrypoint

- [ ] Add stub notification repository in `cmd/api/main.go`
- [ ] Create notification service and handler
- [ ] Pass notification handler to `server.Dependencies`

**Files:** `cmd/api/main.go`

**Validation:** `go build ./...`

## Task 3: Verify all tests pass

- [ ] `go test ./internal/alert/... ./internal/notify/... ./internal/jobs/... -v`
- [ ] `go test ./internal/server/... -v`
- [ ] `go vet ./...`
- [ ] `go build ./...`

**Validation:** all commands exit 0
