# 工程化改造实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 hotkey-server 从手动拼装+Raw SQL 架构升级为 Go + GORM + Fx + PostgreSQL + Redis 工程化体系，并修复全部 20 个代码审查发现的问题

**Architecture:** 
- 目录分层：model → repository(interface) → repository/gormimpl → service → handler + router
- DI：Uber Fx 接管依赖注入和生命周期
- ORM：全部 Raw SQL 改为 GORM builder + Clauses.OnConflict
- 缓存：Cache Aside 模式，Redis 泛型 Cache[T]
- 迁移：goose 版本化迁移
- 测试：gomock 单元测试 + testcontainers 集成测试

**Tech Stack:** Go 1.26 + Gin + GORM v1.31 + Uber Fx + go-redis/v9 + goose/v3 + gomock + testcontainers-go

**OpenSpec Change:** `openspec/changes/2026-07-07-engineering-upgrade/`

**Bug fixes target:** CRITICAL x3 + HIGH x6 + MEDIUM x5 + LOW x6 = 20 issues from code review

---

## Phase 0: Infrastructure & Dependencies

### Task 0.1: Add new Go dependencies

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Install Fx, Redis, goose, gomock**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go get go.uber.org/fx@v1
go get github.com/redis/go-redis/v9
go get github.com/pressly/goose/v3
go install go.uber.org/mock/mockgen@latest
```

- [ ] **Step 2: Verify build**

```bash
make build
```
Expected: binary compiles without error

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "chore: add fx, redis, goose, mock dependencies"
```

### Task 0.2: Create directory skeleton

**Files:**
- Create: `internal/model/doc.go`
- Create: `internal/repository/gormimpl/doc.go`
- Create: `internal/service/doc.go`
- Create: `internal/handler/doc.go`
- Create: `internal/worker/doc.go`
- Create: `internal/cache/doc.go`
- Create: `internal/module/doc.go`
- Create: `internal/fxapp/doc.go`
- Create: `internal/bootstrap/doc.go`
- Create: `internal/pkg/doc.go`

- [ ] **Step: Create directories and placeholder packages**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
for dir in model repository repository/gormimpl service handler worker cache module fxapp bootstrap pkg; do
    mkdir -p "internal/$dir"
    echo "package $(basename $dir)" > "internal/$dir/doc.go"
done
mkdir -p db/migrations
```

- [ ] **Step: Commit**

```bash
git add -A && git commit -m "chore: create new layered directory structure"
```

### Task 0.3: Initialize goose migration

**Files:**
- Create: `db/migrations/000001_create_all_tables.up.sql`
- Create: `db/migrations/000001_create_all_tables.down.sql`

- [ ] **Step: Copy schema as initial migration**

```bash
cp db/schema.sql db/migrations/000001_create_all_tables.up.sql
# Generate down migration
grep "^CREATE TABLE" db/schema.sql | \
  sed 's/CREATE TABLE.*\.\([^ ]*\).*/DROP TABLE IF EXISTS \1;/' > db/migrations/000001_create_all_tables.down.sql
```

- [ ] **Step: Commit**

```bash
git add db/migrations/ && git commit -m "chore: init goose migration from current schema"
```

---

## Phase 1: Model Definitions

### Task 1.1: Create model structs + pkg utilities

**Bug fixes:** Bug #5 (JSONB Scanner), Bug #17 (array util)

**Files:**
- Create: `internal/pkg/jsonb.go`
- Create: `internal/pkg/array.go` (migrate from `internal/database/array.go`)
- Create: `internal/model/user.go`
- Create: `internal/model/monitor.go`
- Create: `internal/model/topic.go`
- Create: `internal/model/event.go`
- Create: `internal/model/hot_event.go`
- Create: `internal/model/content.go`
- Create: `internal/model/notify.go`

- [ ] **Step: Write `internal/pkg/jsonb.go`**

```go
package pkg
import (
    "database/sql/driver"
    "encoding/json"
    "fmt"
)
type JSONB[T any] struct { Data T }
func (j *JSONB[T]) Scan(value any) error {
    if value == nil { return nil }
    bytes, ok := value.([]byte)
    if !ok { return fmt.Errorf("jsonb: expected []byte, got %T", value) }
    return json.Unmarshal(bytes, &j.Data)
}
func (j JSONB[T]) Value() (driver.Value, error) { return json.Marshal(j.Data) }
```

- [ ] **Step: Write all model files, extracted from `internal/database/models.go`** — pure structs, no gorm tags, no TableName()

- [ ] **Step: Verify compilation**

```bash
go build ./internal/...
```

- [ ] **Step: Commit**

```bash
git add internal/pkg/ internal/model/ && git commit -m "feat: add pure model structs with JSONB[T] scanner"
```

### Task 1.2: Create GORM mapping models + conversion helpers

**Files:**
- Create: `internal/repository/gormimpl/model.go`

- [ ] **Step: Write GORM model adapters** — one struct per table with `gorm:"column:..."` tags and `TableName()` methods, using `pkg.JSONB[string]` for JSONB columns

- [ ] **Step: Commit**

```bash
git add internal/repository/gormimpl/model.go && git commit -m "feat: add GORM mapping models"
```

---

## Phase 2: Repository Interfaces

### Task 2.1: Define all repository interfaces

**Bug fixes:** Bug #3, #4 (all interfaces require `ctx context.Context`)

**Files:**
- Create: `internal/repository/user.go`
- Create: `internal/repository/monitor.go`
- Create: `internal/repository/content.go`
- Create: `internal/repository/topic.go`
- Create: `internal/repository/event.go`
- Create: `internal/repository/hot_event.go`
- Create: `internal/repository/digest.go`
- Create: `internal/repository/trend.go`
- Create: `internal/repository/delivery.go`
- Create: `internal/repository/notify.go`
- Create: `internal/repository/annotation.go`
- Create: `internal/repository/run.go`
- Create: `internal/repository/theme.go`

- [ ] **Step: Write each interface** — every method signature must start with `ctx context.Context` as first parameter

```go
// internal/repository/user.go
package repository
import "context"

type UserRepository interface {
    Create(ctx context.Context, email, passwordHash, displayName string) (model.User, error)
    GetByEmail(ctx context.Context, email string) (*model.User, error)
    GetByID(ctx context.Context, id int64) (*model.User, error)
}
```

- [ ] **Step: Verify build**

```bash
go build ./internal/repository/...
```

- [ ] **Step: Commit**

```bash
git add internal/repository/ && git commit -m "feat: add repository interfaces with context propagation"
```

---

## Phase 3: GORM Implementations (Biggest Phase)

### Task 3.1: HotEventRepo

**Bug fixes:** Bug #3 (WithContext on all calls)

**Files:**
- Create: `internal/repository/gormimpl/hot_event.go`

Verify: `go build ./internal/repository/gormimpl/...`

### Task 3.2: Simple CRUD repos

**Bug fixes:** Bug #5 (use JSONB[T] scanner), Bug #11 (use First+ErrRecordNotFound)

**Files:**
- Create: `internal/repository/gormimpl/user.go`
- Create: `internal/repository/gormimpl/event.go`
- Create: `internal/repository/gormimpl/run.go`
- Create: `internal/repository/gormimpl/notify.go`

### Task 3.3: MonitorRepo (dynamic UPDATE → GORM builder)

**File:**
- Create: `internal/repository/gormimpl/monitor.go`

Key: Replace `$1,$2` string concatenation with `db.Model(&m).Where("id = ? AND user_id = ?", id, userID).Updates(map[string]any{})`

### Task 3.4: ContentRepo + Poll (Upsert + Scoring fix)

**Bug fixes:** Bug #2 (scoring writes wrong table), Bug #6 (missing fields)

**Files:**
- Create: `internal/repository/gormimpl/content.go`

Bug #2 fix approach:
```go
type ScorerRepo struct { db *gorm.DB }

func (r *ScorerRepo) UpdateScores(ctx context.Context, postID int64, scores scoring.Scores) error {
    // FIX: Update by post_id instead of the incorrect id
    return r.db.WithContext(ctx).
        Model(&model.MonitorPostHit{}).
        Where("post_id = ?", postID).
        Updates(map[string]any{
            "heat_score":       scores.HeatScore,
            "relevance_score":  scores.RelevanceScore,
            "freshness_score":  scores.FreshnessScore,
            "final_score":      scores.FinalScore,
        }).Error
}
```

### Task 3.5: TopicRepo (Upsert + JOIN)

- Create: `internal/repository/gormimpl/topic.go`

### Task 3.6: Complex query repos (Trend, Digest, Worker, ContentQuery)

**Bug fixes:** Bug #1 (persist snapshots), Bug #8 (no Model+alias), Bug #9 (platform heat)

Bug #1 fix — build_snapshots.go: `BuildTopicSnapshot` must call `repo.SaveTopicSnapshot(snap)` after computing

Bug #8 fix — aggregator: Use explicit `Raw()` + `Scan()` instead of `Model()` + aliased `Select()`

Bug #9 fix — trending heat: Store platform heat value in dedicated column (extend `TrendingItem` struct)

- Create: `internal/repository/gormimpl/trend.go`
- Create: `internal/repository/gormimpl/digest.go`
- Create: `internal/repository/gormimpl/worker.go`

### Task 3.7: Remaining repos (Delivery, Annotation, Theme, HitScorer, Exporter)

- Create: `internal/repository/gormimpl/delivery.go`
- Create: `internal/repository/gormimpl/annotation.go`
- Create: `internal/repository/gormimpl/theme.go`
- Create: `internal/repository/gormimpl/hitscorer.go`
- Create: `internal/repository/gormimpl/exporter.go`

---

## Phase 4: Fx DI Assembly

### Task 4.1: Create Fx Modules

**Files:**
- Create: `internal/module/infra.go`
- Create: `internal/module/auth.go`
- Create: `internal/module/monitor.go`
- Create: `internal/module/topic.go`
- Create: `internal/module/event.go`
- Create: `internal/module/hot_event.go`
- Create: `internal/module/digest.go`
- Create: `internal/module/trend.go`
- Create: `internal/module/notify.go`
- Create: `internal/module/scheduler.go`

### Task 4.2: Wire fxapp/app.go and simplify main.go

**Files:**
- Create: `internal/fxapp/app.go`
- Create: `internal/fxapp/logger.go` (Fx lifecycle-aware logger)
- Create: `internal/fxapp/httpserver.go` (Fx lifecycle-managed HTTP server)
- Modify: `cmd/hotkey/main.go` → `fxapp.NewApp().Run()`

### Task 4.3: Clean up old files

Delete: `internal/app/run.go`, `internal/app/api_server.go`, `internal/database/*repo.go`, `internal/database/*query.go`, `internal/database/models.go`, `internal/database/array.go`, `internal/database/hitscorer.go`, `internal/database/exporter.go`, `internal/database/worker_*.go`

Keep: `internal/database/database.go` (Open function), `internal/database/bootstrap.go` (migration init)

---

## Phase 5: Redis Cache

### Task 5.1: Generic cache implementation

- Create: `internal/cache/cache.go` — `Cache[T any]` with Get/Set/Del

### Task 5.2: Business caches

- Create: `internal/cache/hot_event_cache.go`
- Create: `internal/cache/topic_cache.go`
- Create: `internal/cache/monitor_cache.go`

---

## Phase 6: Worker Fx Lifecycle

### Task 6.1: Migrate workers to Fx

- Create: `internal/worker/poll_worker.go`
- Create: `internal/worker/digest_worker.go`
- Create: `internal/worker/cleanup_worker.go`
- Create: `internal/worker/snapshot_worker.go`

Bug #20 fix: Remove duplicate `signal.Notify`, let main Fx lifecycle handle shutdown

---

## Phase 7: Testing

### Task 7.1: Generate mocks

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
for f in internal/repository/*.go; do
    name=$(basename $f .go)
    mockgen -source=$f -destination=internal/repository/mock/${name}_mock.go
done
```

### Task 7.2: Service unit tests (key services)

- Create: `internal/service/auth_test.go`
- Create: `internal/service/monitor_test.go`
- Create: `internal/service/hot_event_test.go`
- Create: `internal/service/trend_test.go`
- Create: `internal/service/digest_test.go`

### Task 7.3: Integration test setup

- Create: `tests/integration/suite.go` (testcontainers PostgreSQL + Redis)

### Task 7.4: Update Makefile

```makefile
.PHONY: test-unit test-integration test-all
test-unit:
    go test ./internal/service/... ./internal/handler/... -v -count=1
test-integration:
    go test ./tests/integration/... -v -count=1 -tags=integration
test-all: test-unit test-integration
```

---

## Bug Fix Verification Checklist

| # | Bug | Severity | Verified In | Method |
|---|-----|----------|-------------|--------|
| 1 | Trend not persisted | CRITICAL | Task 3.6 | Unit test asserts snapshot row created |
| 2 | Wrong table in scoring | CRITICAL | Task 3.4 | Integration test: poll flow + assert correct id |
| 3 | No ctx in HotEventRepo | CRITICAL | Task 3.1 | Compiler: interface requires ctx |
| 4 | No ctx in other repos | HIGH | Task 3.2-3.7 | Compiler: interface requires ctx |
| 5 | Silent JSON errors | HIGH | Task 1.1 | JSONB[T] Scanner returns errors |
| 6 | Missing fields UpsertHit | HIGH | Task 3.4 | Code review: all fields set |
| 7 | Silent error handler | HIGH | Task 6.1 | Code review: error logged |
| 8 | Model+alias SQL | HIGH | Task 3.6 | Uses Raw+Scan |
| 9 | Cross-platform heat=0 | HIGH | Task 3.6 | Dedicated heat column |
| 10 | AppError nil guard | MEDIUM | Task 6.1 | Guard removed |
| 11 | AuthRepo zero check | MEDIUM | Task 3.2 | First()+ErrRecordNotFound |
| 12 | BuildSnapshots no ctx | MEDIUM | Task 3.6 | Interface requires ctx |
| 13 | Duplicate delivery | MEDIUM | Task 3.2 | Transactional approach |
| 14 | Ignored AddPlatform | MEDIUM | Task 3.1 | Error propagated |
| 15-20 | Various LOW | LOW | Throughout | Code review confirmed |
