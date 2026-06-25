# Tasks: L4 业务闭环 — Repository + Worker 去 stub

## Task 1: content repository implementation

**Files:** `internal/database/contentrepo.go`

- [ ] 1.1 `ContentRepo` struct with `*sql.DB` field
- [ ] 1.2 `NewContentRepo(db *sql.DB) *ContentRepo`
- [ ] 1.3 `UpsertPost(ctx, post) (int64, error)` — INSERT ... ON CONFLICT returning id
- [ ] 1.4 `GetPostByPlatformID(ctx, platform, platformPostID) (*NormalizedPost, error)`
- [ ] 1.5 `UpsertHit(ctx, hit) error` — INSERT ... ON CONFLICT
- [ ] 1.6 `GetHitsByMonitor(ctx, monitorID) ([]MonitorHit, error)`

**Validation:** `go build ./internal/database/...`

## Task 2: topic repository implementation

**Files:** `internal/database/topicrepo.go`

- [ ] 2.1 `TopicRepo` struct with `*sql.DB` field
- [ ] 2.2 `NewTopicRepo(db *sql.DB) *TopicRepo`
- [ ] 2.3 `UpsertTopic(ctx, monitorID, topic) (int64, error)` — INSERT ... ON CONFLICT
- [ ] 2.4 `AddPostToTopic(ctx, topicID, postID, membershipScore) error`
- [ ] 2.5 `ListByMonitor(ctx, monitorID) ([]TopicSummary, error)` — JOIN topic_posts

**Validation:** `go build ./internal/database/...`

## Task 3: trend repository implementation

**Files:** `internal/database/trendrepo.go`

- [ ] 3.1 `TrendRepo` struct with `*sql.DB` field
- [ ] 3.2 `NewTrendRepo(db *sql.DB) *TrendRepo`
- [ ] 3.3 `SaveTopicSnapshot(ctx, snap) error`
- [ ] 3.4 `SaveMonitorSnapshot(ctx, snap) error`
- [ ] 3.5 `GetPreviousTopicHeat(ctx, topicID) (float64, error)`

**Validation:** `go build ./internal/database/...`

## Task 4: query service implementations

**Files:** `internal/database/contentquery.go`, `internal/database/topicquery.go`, `internal/database/trendquery.go`

- [ ] 4.1 `ContentQueryService` implementing `content.PostQueryService`
- [ ] 4.2 `TopicQueryService` implementing `topic.TopicQueryService`
- [ ] 4.3 `TrendQueryService` implementing `trend.TrendQueryService`

**Validation:** `go build ./internal/database/...`

## Task 5: worker job wiring adapters

**Files:** `internal/jobs/adapters.go`

- [ ] 5.1 `XConnectorAdapter` wrapping `x.Client` as `PlatformConnector`
- [ ] 5.2 `ScorerAdapter` wrapping `scoring.Service` as `HitScorer`
- [ ] 5.3 `DBPostCandidateProvider` implementing `PostCandidateProvider`
- [ ] 5.4 `TopicPersisterAdapter` wrapping `topic.Repository` as `TopicPersister`
- [ ] 5.5 `DBTopicProvider` implementing `TopicProvider`
- [ ] 5.6 `DBDeliveryRepository` implementing `DeliveryRepository`
- [ ] 5.7 `DBUserEmailLookup` implementing `UserEmailLookup`

**Validation:** `go build ./internal/jobs/...`

## Task 6: main.go stub removal + real wiring

**Files:** `cmd/api/main.go`

- [ ] 6.1 Remove `stubPostQueryService`, `stubTopicQueryService`, `stubTrendQueryService`
- [ ] 6.2 Remove `stubDeliveryRepo`, `stubMailer`, `stubUserEmailLookup`
- [ ] 6.3 Wire real repositories in `runAPI()` (non-smoke path)
- [ ] 6.4 Wire real repositories in `runWorker()`
- [ ] 6.5 Wire real PollMonitorJob with x.Client + scoring
- [ ] 6.6 Wire real AggregateTopicsJob, BuildSnapshotsJob
- [ ] 6.7 Preserve `SMOKE_TEST=1` path with smoke stubs

**Validation:** `go build ./cmd/api/...`

## Task 7: tests

**Files:** `internal/database/contentrepo_test.go`, `internal/database/topicrepo_test.go`, `internal/database/trendrepo_test.go`

- [ ] 7.1 content repo unit tests (interface compliance)
- [ ] 7.2 topic repo unit tests (interface compliance)
- [ ] 7.3 trend repo unit tests (interface compliance)

**Validation:** `go test ./internal/database/... -v`

## Task 8: final validation

- [ ] 8.1 `make test`
- [ ] 8.2 `make validate`
- [ ] 8.3 `go test ./internal/jobs/... -v`
