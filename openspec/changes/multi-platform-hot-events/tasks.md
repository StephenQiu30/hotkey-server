## 1. Project Refactor — Connector Interface Extraction

- [ ] 1.1 Create `internal/connector/` package with `search.go`, `trending.go`, `types.go`
- [ ] 1.2 Migrate `PlatformConnector` interface from `internal/jobs/poll_monitor.go` to `internal/connector/search.go` as `Searcher`
- [ ] 1.3 Define `TrendingCollector` interface in `internal/connector/trending.go`
- [ ] 1.4 Define shared types (`SearchResult`, `TrendingItem`, `PostResult`) in `internal/connector/types.go`
- [ ] 1.5 Update `internal/jobs/poll_monitor.go` to remove old interface definition, import `connector.Searcher`
- [ ] 1.6 Update `internal/jobs/adapters.go` to reference new `connector` types
- [ ] 1.7 Fix all import paths across the codebase
- [ ] 1.8 Run `make build` and `make test` to confirm no regressions

## 2. Project Refactor — Job Package Separation & Wiring Registration

- [ ] 2.1 Refactor `internal/app/worker_jobs.go` to registration pattern: each job package exposes `Register(r *jobs.Runner, db *gorm.DB)`
- [ ] 2.2 Keep existing X poll_monitor, aggregate_topics, build_snapshots, dispatch_notifications, publish_daily_topics registration unchanged
- [ ] 2.3 Create `internal/app/routes.go` for new HTTP route registration
- [ ] 2.4 Run `make build` and `make test` to confirm no regressions

## 3. Database — New Tables & Migration

- [ ] 3.1 Add GORM models for `HotEvent` and `HotEventPlatform` in `internal/database/models.go`
- [ ] 3.2 Create `internal/database/repositories/hot_event.go` with repository implementation
- [ ] 3.3 Add auto-migration for new tables in database init
- [ ] 3.4 Run `make build` to confirm new models compile

## 4. HotEvent Domain — Entity & Service

- [ ] 4.1 Create `internal/hotevent/model.go` with `HotEvent` struct, status constants, sentinel errors
- [ ] 4.2 Create `internal/hotevent/repository.go` with repository interface (Create, GetByID, List, Update, Archive)
- [ ] 4.3 Create `internal/hotevent/service.go` with `HeatScore` computation and lifecycle management
- [ ] 4.4 Create `internal/hotevent/service_test.go` with tests for HeatScore computation
- [ ] 4.5 Run `make test` to confirm tests pass

## 5. Platform Clients — Weibo

- [ ] 5.1 Create `internal/platform/weibo/client.go` — implement `TrendingCollector`
- [ ] 5.2 Parse `https://weibo.com/ajax/side/hotSearch` JSON response into `TrendingItem`
- [ ] 5.3 Create `internal/platform/weibo/client_test.go` with test using sample response
- [ ] 5.4 Run `make test` to confirm tests pass

## 6. Platform Clients — Zhihu

- [ ] 6.1 Create `internal/platform/zhihu/client.go` — implement `TrendingCollector`
- [ ] 6.2 Parse `https://www.zhihu.com/api/v3/feed/topstory/hot-lists/total` JSON response into `TrendingItem`
- [ ] 6.3 Create `internal/platform/zhihu/client_test.go` with test using sample response
- [ ] 6.4 Run `make test` to confirm tests pass

## 7. Platform Clients — Baidu

- [ ] 7.1 Create `internal/platform/baidu/client.go` — implement `TrendingCollector`
- [ ] 7.2 Parse `https://top.baidu.com/board?tab=realtime` HTML into `TrendingItem`
- [ ] 7.3 Create `internal/platform/baidu/client_test.go` with test using sample HTML
- [ ] 7.4 Run `make test` to confirm tests pass

## 8. Trending Collector Job

- [ ] 8.1 Create `internal/collector/job.go` — `TrendingCollectorJob` struct with `Register()` and `Run()`
- [ ] 8.2 Create `internal/collector/adapters.go` — wire weibo/zhihu/baidu clients into `TrendingCollector` interface
- [ ] 8.3 Register `collect_trending` job in `app/worker_jobs.go`
- [ ] 8.4 Create `internal/collector/job_test.go` with tests (mock adapter)
- [ ] 8.5 Run `make build` and `make test` to confirm

## 9. HotEvent Aggregator

- [ ] 9.1 Create `internal/aggregator/matcher.go` — `EventMatcher` with cosine similarity + keyword overlap algorithm
- [ ] 9.2 Create `internal/aggregator/matcher_test.go` with tests for matching
- [ ] 9.3 Create `internal/aggregator/job.go` — `HotEventAggregatorJob` struct with `Register()` and `Run()`
- [ ] 9.4 Create `internal/aggregator/job_test.go` with integration-style tests
- [ ] 9.5 Register `aggregate_events` job in `app/worker_jobs.go`
- [ ] 9.6 Run `make test` to confirm

## 10. Data Cleanup Job

- [ ] 10.1 Create `internal/cleanup/job.go` — `CleanupJob` with configurable retention policy
- [ ] 10.2 Implement platform_posts cleanup (delete older than DATA_RETENTION_DAYS, skip posts referenced by active HotEvents)
- [ ] 10.3 Implement HotEvent archival (set status=archived after HOT_EVENT_ARCHIVE_DAYS)
- [ ] 10.4 Create `internal/cleanup/job_test.go`
- [ ] 10.5 Register `cleanup_data` job in `app/worker_jobs.go`
- [ ] 10.6 Run `make test` to confirm

## 11. HotEvent HTTP API

- [ ] 11.1 Create `internal/http/handler/trending.go` — GET `/api/v1/trending` handler
- [ ] 11.2 Create GET `/api/v1/hot-events` handler with filters (status, platform, sort, limit)
- [ ] 11.3 Create GET `/api/v1/hot-events/:id` handler with 404 for not found
- [ ] 11.4 Create GET `/api/v1/hot-events/:id/posts` handler
- [ ] 11.5 Register new routes in `internal/app/routes.go`
- [ ] 11.6 Add API config for trending endpoint to `internal/config/config.go`
- [ ] 11.7 Run `make build` to confirm

## 12. Daily Digest Extension

- [ ] 12.1 Extend `publish_daily_topics` to include HotEvents in daily digest content
- [ ] 12.2 Add `## 各平台热点汇总` section to daily digest Markdown
- [ ] 12.3 Extend frontmatter with event_count, platforms fields
- [ ] 12.4 Add HotEvent idempotent export (similar to topic_daily_exports)
- [ ] 12.5 Run `make build` and `make test`

## 13. Integration Smoke Test

- [ ] 13.1 Add trending and hot-events smoke checks to `scripts/smoke-api.sh`
- [ ] 13.2 Run full CI pipeline locally: `make build && make lint && make test`
- [ ] 13.3 Verify binary starts and new endpoints respond
