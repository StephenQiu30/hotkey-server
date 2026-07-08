# Task 5 Report — Move HTTP handlers to controller/, add model/vo/

**Status:** DONE

## Summary

Moved all HTTP handler files from `internal/platform/http/` to `internal/controller/`, with package renamed from `http` to `controller`. Created `internal/model/vo/` package for response view objects (VO types). Updated all imports and references across the codebase.

## Files Changed

### Created (controller/)
- `internal/controller/auth.go` — handler logic, uses `vo.UserData`/`vo.LoginData`
- `internal/controller/auth_helpers.go` — userIDFromCtx helper, imports `platformhttp.UserIDKey`
- `internal/controller/content.go` — content handler (unchanged logic)
- `internal/controller/health.go` — health handler, uses `vo.HealthBody`
- `internal/controller/monitor.go` — monitor CRUD handlers, uses `vo.MonitorData`
- `internal/controller/notify.go` — notification handlers, uses `vo.NotificationData`
- `internal/controller/report.go` — report handlers, unchanged logic
- `internal/controller/request.go` — request DTOs + swagger response types, imports `vo.*`
- `internal/controller/response.go` — `RespondOK/RespondCreated/RespondPage/respondError/respondInternalError` helpers using `vo.ResponseBody`/`vo.PageBody` and `platformhttp.ErrorBody`
- `internal/controller/route.go` — router config + wiring, references `platformhttp.*` middleware
- `internal/controller/topic.go` — topic handler (unchanged)
- `internal/controller/trend.go` — trend handler (unchanged)
- `internal/controller/trending.go` — trending/hot-event handlers (unchanged)

### Created (model/vo/)
- `internal/model/vo/common.go` — `ResponseBody`, `PageBody`
- `internal/model/vo/auth.go` — `UserData`, `LoginData`
- `internal/model/vo/monitor.go` — `MonitorData`
- `internal/model/vo/health.go` — `HealthBody`
- `internal/model/vo/notify.go` — `NotificationData`, `MarkNotificationReadData`

### Deleted (from platform/http/)
- `auth.go`, `auth_helpers.go`, `content.go`, `health.go`, `monitor.go`, `notify.go`, `report.go`, `response.go`, `router.go`, `swagger_types.go`, `topic.go`, `trend.go`, `trending.go`

### Kept (in platform/http/)
- `errors.go`, `middleware.go`, `accesslog.go`

### Modified
- `internal/fxapp/app.go` — `platformhttp.NewRouter/Config/HotEventManager` → `controller.NewRouter/Config/HotEventManager`
- `tests/testutil/router.go` — `platformhttp.NewRouter/Config` → `controller.NewRouter/Config`
- `tests/unit/platform/http/router_test.go` — mixed `controller.*` for router/config/respond helpers, `platformhttp.*` for middleware/error types
- `tests/unit/platform/http/report_test.go` — `platformhttp.ReportService` → `controller.ReportService`, `platformhttp.NewRouter/Config` → `controller.NewRouter/Config`

## Interfaces Moved to controller/
- `HotEventManager` (trending.go)
- `ReportService` (report.go)
- `MonitorGetter` (monitor.go)
- `TopicMonitorIDGetter` (trend.go)

## Build/Test Results

- `go build ./...` — PASS
- `go test ./...` — ALL TESTS PASS

## Git Commits

- `refactor: move HTTP handlers to controller/, add model/vo/`
