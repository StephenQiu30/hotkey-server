# Proposal: L4 业务闭环 — Repository + Worker 去 stub

## Summary

Remove all stub implementations from the production code path in `cmd/api/main.go` and replace them with real PostgreSQL-backed repository implementations and properly wired worker jobs.

## Scope

### In Scope

- content repository (PostRepository, HitRepository) backed by PostgreSQL
- topic repository (UpsertTopic, AddPostToTopic, ListByMonitor) backed by PostgreSQL
- trend repository (SaveTopicSnapshot, SaveMonitorSnapshot, GetPreviousTopicHeat) backed by PostgreSQL
- Query service implementations for content, topic, trend that read from DB
- Worker job wiring: PollMonitorJob with real x.Client + scoring, AggregateTopicsJob, BuildSnapshotsJob, DispatchJob
- Remove stub structs from production path in main.go
- Preserve SMOKE_TEST=1 in-memory stub path

### Out of Scope

- sqlc code generation (L3 / STE-299)
- Redis integration (L3 / STE-299)
- asynq queue integration (L3 / STE-299)
- New HTTP endpoints or OpenAPI changes
- Schema migrations (tables assumed to exist)

## Non-Goals

- Performance optimization of queries
- Caching layer
- Async job queue (deferred to L3)

## Impact

- `internal/database/` — new repository files
- `internal/content/` — new query service impl
- `internal/topic/` — new query service impl
- `internal/trend/` — new query service impl
- `internal/jobs/` — new adapter types
- `cmd/api/main.go` — stub removal, real wiring
