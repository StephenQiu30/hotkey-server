# Proposal: STE-284 提醒通知闭环

## Summary

Wire the already-implemented alert rule engine, notification center, and email dispatch job into the HTTP router and main entrypoint so that the notification system is accessible end-to-end.

## Normative files changed

- `internal/server/router.go` — add `NotificationHandler` field to `Dependencies`, mount notification routes
- `cmd/api/main.go` — wire notification handler with stub repository (same pattern as auth/monitor)

## Non-goals

- PostgreSQL repository implementations (deferred to DB integration phase)
- Concrete Mailer/SMTP implementation (plan says: use fake mailer for now)
- Notification scheduler integration (dispatch job wiring into cron)
- Frontend notification UI (Plan 006 scope)

## Scope

The business logic for Plan 005 Tasks 1-3 is already implemented:
- `internal/alert/` — rule engine with 3 default rules, 5 tests passing
- `internal/notify/` — notification service, HTTP handler, mailer interface, 9 tests passing
- `internal/jobs/dispatch_notifications.go` — dispatch job with delivery audit, 4 tests passing
- `db/schema.sql` — alerts, user_notifications, email_deliveries tables present

The only missing piece is router wiring: the notification handler has `RegisterRoutes` but is not mounted in the main router.
