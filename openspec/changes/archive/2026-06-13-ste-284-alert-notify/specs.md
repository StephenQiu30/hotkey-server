# Specs: STE-284 提醒通知闭环

## Requirements

### REQ-1: Notification routes MUST be mounted in the main router

- `Dependencies` struct SHALL include a `NotificationHandler http.Handler` field
- `NewRouter` SHALL mount `GET /api/v1/notifications` and `POST /api/v1/notifications/{id}/read` via the notification handler
- Routes SHALL require auth middleware (same pattern as topic/trend/post routes)

### REQ-2: Main entrypoint MUST wire notification handler

- `cmd/api/main.go` SHALL create a notification service with a stub repository
- The notification handler SHALL be passed to `server.Dependencies`

### REQ-3: Existing tests MUST continue to pass

- `go test ./internal/alert/... ./internal/notify/... ./internal/jobs/...` SHALL pass
- `go vet ./...` SHALL pass
- `go build ./...` SHALL succeed

## Validation evidence

- `make test` passes all 18 tests across alert, notify, and jobs packages
- Router test verifies notification routes are accessible (or require auth)
