# Design: STE-284 提醒通知闭环

## Goals

Connect the existing notification handler to the HTTP router following the established pattern.

## Non-goals

- Database-backed repository implementation
- Real mailer/SMTP integration
- Dispatch job scheduling

## Contracts

- `server.Dependencies` gets a new `NotificationHandler http.Handler` field
- Notification routes require `AuthMiddleware` (consistent with topic/trend/post)
- `cmd/api/main.go` uses a stub notification repo (same pattern as auth/monitor stubs)

## State flow

1. Request arrives at `GET /api/v1/notifications?user_id=X`
2. Auth middleware validates token (currently rejects with 401)
3. Notification handler delegates to service → repository
4. Response: JSON array of unread notifications

## Failure paths

- Missing `user_id` → 400 Bad Request
- Invalid notification ID → 404 Not Found
- Wrong user ownership → 404 Not Found (ErrNotOwned)

## Rollback impact

None — adding a new handler field and mounting routes is additive.
