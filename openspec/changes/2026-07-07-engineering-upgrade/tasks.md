# Engineering Upgrade — Tasks

> **Plan also stored at:** `docs/superpowers/plans/2026-07-07-engineering-upgrade.md`
> **Bug fixes reference:** Code review findings #1-#20 from review report
> **Design:** `docs/superpowers/specs/2026-07-07-engineering-upgrade-design.md`

## Phase 0: Infrastructure & Dependencies

### Task 0.1: Add new Go dependencies

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Install Fx, Redis, goose, gomock**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server

# DI container
go get go.uber.org/fx@v1

# Redis client
go get github.com/redis/go-redis/v9

# Migration tool
go get github.com/pressly/goose/v3

# Mock generator
go install go.uber.org/mock/mockgen@latest

# Test containers
go get github.com/testcontainers/testcontainers-go
```

- [ ] **Step 2: Verify build**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
make build
```
Expected: binary compiles without error

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add fx, redis, goose, testcontainers dependencies"
```

### Task 0.2: Create new directory skeleton

**Files:**
- Create: `internal/model/` (empty package)
- Create: `internal/repository/` (empty package)
- Create: `internal/repository/gormimpl/` (empty package)
- Create: `internal/service/` (empty package)
- Create: `internal/handler/` (empty package but keep existing handler/)
- Create: `internal/middleware/` (empty)
- Create: `internal/worker/` (empty)
- Create: `internal/cache/` (empty package)
- Create: `internal/module/` (empty package)
- Create: `internal/fxapp/` (empty package)
- Create: `internal/bootstrap/` (placeholder)
- Create: `internal/pkg/` (placeholder)
- Create: `db/migrations/` (placeholder)

- [ ] **Step: Create directories**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
mkdir -p internal/model
mkdir -p internal/repository/gormimpl
mkdir -p internal/service
mkdir -p internal/handler
mkdir -p internal/middleware
mkdir -p internal/worker
mkdir -p internal/cache
mkdir -p internal/module
mkdir -p internal/fxapp
mkdir -p internal/bootstrap
mkdir -p internal/pkg
mkdir -p db/migrations
```

- [ ] **Step: Add package placeholder files**

```bash
# Touch go package files (one per directory to establish packages)
for dir in internal/model internal/repository internal/repository/gormimpl internal/service internal/handler internal/middleware internal/worker internal/cache internal/module internal/fxapp internal/bootstrap internal/pkg; do
    echo "package $(basename $dir | sed 's/gormimpl/gormimpl/')" > "$dir/doc.go"
done
```

- [ ] **Step: Commit**

```bash
git add internal/model internal/repository internal/service internal/handler internal/middleware internal/worker internal/cache internal/module internal/fxapp internal/bootstrap internal/pkg db/migrations
git commit -m "chore: create new layered directory structure skeleton"
```

### Task 0.3: Initialize goose migration directory

**Files:**
- Create: `db/migrations/000001_create_all_tables.up.sql`
- Create: `db/migrations/000001_create_all_tables.down.sql`

- [ ] **Step: Copy current schema.sql as initial migration**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server

# Copy current schema as initial up migration
cp db/schema.sql db/migrations/000001_create_all_tables.up.sql

# Create empty down migration
echo "-- Down migration: drops all tables" > db/migrations/000001_create_all_tables.down.sql
# Append DROP TABLE statements from schema
grep "^CREATE TABLE" db/schema.sql | sed 's/CREATE TABLE \(IF NOT EXISTS \)\?\([^ ]*\).*/DROP TABLE IF EXISTS \2;/' >> db/migrations/000001_create_all_tables.down.sql
```

- [ ] **Step: Commit**

```bash
git add db/migrations/
git commit -m "chore: initialize goose migration with current schema"
```

---

## Phase 1: Model Definitions + GORM Adapters

### Task 1.1: Create pure model structs

**Bug fixes embedded:** Bug #5 (JSONB Scanner), Bug #17 (array serialization)

**Files:**
- Create: `internal/model/user.go`
- Create: `internal/model/monitor.go`
- Create: `internal/model/topic.go`
- Create: `internal/model/event.go`
- Create: `internal/model/hot_event.go`
- Create: `internal/model/content.go`
- Create: `internal/model/notify.go`
- Create: `internal/pkg/jsonb.go` — JSONB[T] generic Scanner/Valuer
- Create: `internal/pkg/array.go` — migrate from `internal/database/array.go`

- [ ] **Step 1: Create JSONB[T] generic type**

```go
// internal/pkg/jsonb.go
package pkg

import (
    "database/sql/driver"
    "encoding/json"
    "fmt"
)

// JSONB is a generic type for PostgreSQL JSONB columns.
type JSONB[T any] struct {
    Data T
}

func (j *JSONB[T]) Scan(value any) error {
    if value == nil {
        return nil
    }
    bytes, ok := value.([]byte)
    if !ok {
        return fmt.Errorf("jsonb: expected []byte, got %T", value)
    }
    return json.Unmarshal(bytes, &j.Data)
}

func (j JSONB[T]) Value() (driver.Value, error) {
    return json.Marshal(j.Data)
}
```

- [ ] **Step 2: Create model/monitor.go** (example — repeat pattern for all models)

```go
// internal/model/monitor.go
package model

import (
    "time"
    "github.com/StephenQiu30/hotkey-server/internal/pkg"
)

type KeywordMonitor struct {
    ID                   int64
    UserID               int64
    Name                 string
    QueryText            string
    Language             string
    Region               string
    Status               string
    PollIntervalMinutes   int
    AlertEnabled         bool
    AlertThresholdConfig pkg.JSONB[map[string]any]
    LastPolledAt         *time.Time
    CreatedAt            time.Time
    UpdatedAt            time.Time
}
```

- [ ] **Step 3: Repeat for all models listed above**

For each model file, extract the fields from `internal/database/models.go` into a pure struct (no gorm tag, no TableName).

- [ ] **Step 4: Verify compilation**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go build ./internal/model/...
go build ./internal/pkg/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/model/ internal/pkg/
git commit -m "feat: add pure model structs with JSONB[T] scanner"
```

### Task 1.2: Create GORM mapping models

**Files:**
- Create: `internal/repository/gormimpl/model.go` — GORM-tagged models mapping to existing tables

- [ ] **Step: Create GORM model adapters**

```go
// internal/repository/gormimpl/model.go
package gormimpl

import (
    "time"
    "github.com/StephenQiu30/hotkey-server/internal/pkg"
    "gorm.io/gorm"
)

// GORM models mirror database schema exactly.
// Conversion functions handle model ↔ business struct mapping.

type User struct {
    ID           int64     `gorm:"column:id;primaryKey"`
    Email        string    `gorm:"column:email"`
    PasswordHash string    `gorm:"column:password_hash"`
    DisplayName  string    `gorm:"column:display_name"`
    Status       string    `gorm:"column:status"`
    PlanType     string    `gorm:"column:plan_type"`
    CreatedAt    time.Time `gorm:"column:created_at"`
    UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (User) TableName() string { return "users" }

// KeywordMonitor GORM model — maps to keyword_monitors table
type KeywordMonitor struct {
    ID                   int64              `gorm:"column:id;primaryKey"`
    UserID               int64              `gorm:"column:user_id"`
    Name                 string             `gorm:"column:name"`
    QueryText            string             `gorm:"column:query_text"`
    Language             string             `gorm:"column:language"`
    Region               string             `gorm:"column:region"`
    Status               string             `gorm:"column:status"`
    PollIntervalMinutes  int                `gorm:"column:poll_interval_minutes"`
    AlertEnabled         bool               `gorm:"column:alert_enabled"`
    AlertThresholdConfig pkg.JSONB[string]  `gorm:"column:alert_threshold_config;type:jsonb"`
    LastPolledAt         *time.Time         `gorm:"column:last_polled_at"`
    CreatedAt            time.Time          `gorm:"column:created_at"`
    UpdatedAt            time.Time          `gorm:"column:updated_at"`
}

func (KeywordMonitor) TableName() string { return "keyword_monitors" }

// ... all other GORM models follow the same pattern

// Conversion helpers
func ToMonitorModel(m model.KeywordMonitor) KeywordMonitor { /* ... */ }
func FromMonitorModel(m KeywordMonitor) model.KeywordMonitor { /* ... */ }
```

- [ ] **Step: Verify compilation**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go build ./internal/repository/gormimpl/...
```

- [ ] **Step: Commit**

```bash
git add internal/repository/gormimpl/model.go
git commit -m "feat: add GORM mapping models with conversion helpers"
```

---

## Phase 2: Repository Interfaces

### Task 2.1: Define all repository interfaces

**Bug fixes embedded:** Bug #3, #4 (all interfaces must require `ctx context.Context`)

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

- [ ] **Step: Create interfaces with ctx requirement (example — monitor.go)**

```go
// internal/repository/monitor.go
package repository

import (
    "context"
    "github.com/StephenQiu30/hotkey-server/internal/monitor"
)

type MonitorRepository interface {
    Create(ctx context.Context, userID int64, input monitor.CreateMonitorInput) (monitor.Monitor, error)
    GetByID(ctx context.Context, id int64) (*monitor.Monitor, error)
    ListByUser(ctx context.Context, userID int64) ([]monitor.Monitor, error)
    Update(ctx context.Context, id int64, userID int64, input monitor.UpdateMonitorInput) (monitor.Monitor, error)
    ListActiveIDs(ctx context.Context) ([]int64, error)
}
```

- [ ] **Step: Verify compilation**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go build ./internal/repository/...
```

- [ ] **Step: Commit**

```bash
git add internal/repository/
git commit -m "feat: add repository interfaces with context propagation"
```

---

## Phase 3: GORM Implementations (Core Migration)

### Task 3.1: Migrate HotEventRepo (good template to establish pattern)

**Bug fixes embedded:** Bug #3 (add context to all methods), Bug #5 (use JSONB scanner)

**Files:**
- Create: `internal/repository/gormimpl/hot_event.go`
- Keep (for now): `internal/database/hoteventrepo.go`

- [ ] **Step: Create gormimpl/hot_event.go with WithContext + interface pattern**

```go
// internal/repository/gormimpl/hot_event.go
package gormimpl

import (
    "context"
    "time"
    // ...
)

type HotEventRepo struct {
    db *gorm.DB
}

func NewHotEventRepo(db *gorm.DB) *HotEventRepo {
    return &HotEventRepo{db: db}
}

func (r *HotEventRepo) GetByID(ctx context.Context, id int64) (*hotevent.HotEvent, error) {
    var model HotEvent
    if err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error; err != nil {
        if err == gorm.ErrRecordNotFound {
            return nil, hotevent.ErrNotFound
        }
        return nil, err
    }
    return FromHotEvent(&model), nil
}

// ... all existing methods, now with WithContext(ctx)
```

- [ ] **Step: Verify compilation**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
go build ./internal/repository/gormimpl/...
```

- [ ] **Step: Commit**

```bash
git add internal/repository/gormimpl/hot_event.go
git commit -m "feat: migrate HotEventRepo to gormimpl with context propagation"
```

### Task 3.2: Migrate simple CRUD repos (auth, event, run, notify, topic_event_linker, knowledge_writeback)

**Bug fixes embedded:** Bug #11 (AuthRepo use First()+ErrRecordNotFound instead of ID==0 check)

**Files:**
- Create: `internal/repository/gormimpl/user.go`
- Create: `internal/repository/gormimpl/event.go`
- Create: `internal/repository/gormimpl/run.go`
- Create: `internal/repository/gormimpl/notify.go`
- Create: `internal/repository/gormimpl/topic_event_linker.go`
- Create: `internal/repository/gormimpl/knowledge_writeback.go`

- [ ] **Step: Migrate each file one by one**

Pattern for each:
1. Define conversion functions (business model ↔ GORM model)
2. Implement repository interface using GORM builder
3. Use `.WithContext(ctx)` on every call
4. Use `.First()` instead of `Raw().Scan()` for single-row queries
5. Use `.Find()` for multi-row queries
6. Use `.Create()` for inserts
7. Use `.Updates(map)` for updates
8. Use `.Delete()` for deletes

- [ ] **Step: After each file, verify build**

```bash
go build ./internal/repository/gormimpl/...
```

- [ ] **Step: Commit (one commit per file for clean history)**

```bash
git add internal/repository/gormimpl/user.go
git commit -m "feat: migrate user repo to gormimpl with GORM builder"
```

### Task 3.3: Migrate MonitorRepo (biggest challenge)

**Bug fixes embedded:** Bug #5 (JSONB scanner eliminates silent discard)

**Files:**
- Create: `internal/repository/gormimpl/monitor.go`

Key differences from current:
- Replace `INSERT ... RETURNING` with `db.Create()`
- Replace `SELECT ... WHERE id = ? ...Row().Scan(...)` with `db.Where("id = ?", id).First(&model)`
- Replace dynamic UPDATE with `db.Model(&model).Where(...).Updates(map[string]any{})`
- All with `.WithContext(ctx)`

- [ ] **Step: Commit**

```bash
git add internal/repository/gormimpl/monitor.go
git commit -m "feat: migrate MonitorRepo - eliminate raw SQL dynamic building"
```

### Task 3.4: Migrate ContentRepo + worker_poll (Upsert focus)

**Bug fixes embedded:** Bug #2 (scoring writes wrong table — fix HitID mapping), Bug #6 (add missing relevance_score)

**Files:**
- Create: `internal/repository/gormimpl/content.go`

Key changes:
- Replace `INSERT ... ON CONFLICT DO UPDATE ... RETURNING` with `db.Clauses(clause.OnConflict{...}).Create()`
- Bug #2 fix: Add `GetHitByPostID(ctx, monitorID, postID)` that queries `monitor_post_hits` by `(monitor_id, post_id)` to get actual `id`
- Bug #6 fix: Always set `relevance_score` and `first_seen_at` in UpsertHit

- [ ] **Step: Commit**

```bash
git add internal/repository/gormimpl/content.go
git commit -m "feat: migrate ContentRepo with Upsert via Clauses.OnConflict"
```

### Task 3.5: Migrate TopicRepo (Upsert + JOIN)

**Files:**
- Create: `internal/repository/gormimpl/topic.go`

Key changes:
- Replace `INSERT ON CONFLICT ... RETURNING` with `Clauses.OnConflict`
- Replace LEFT JOIN + GROUP BY with GORM `Joins` + `Select`

- [ ] **Step: Commit**

```bash
git add internal/repository/gormimpl/topic.go
git commit -m "feat: migrate TopicRepo with GORM builder"
```

### Task 3.6: Migrate complex query repos (digest, trend, worker, content)

**Bug fixes embedded:** Bug #1 (trend snapshots not persisted — add save calls), Bug #12 (add context to interface)

**Files:**
- Create: `internal/repository/gormimpl/digest.go`
- Create: `internal/repository/gormimpl/trend.go`
- Create: `internal/repository/gormimpl/worker.go`

Bug #1 fix — in trendrepo:
```go
func (r *TrendRepo) BuildTopicSnapshot(ctx context.Context, topicID int64) (*trend.TrendSnapshot, error) {
    // ... compute snapshot ...
    snap := &topic.Snapshot{...}
    // BUG FIX: Actually save the snapshot
    if err := r.db.WithContext(ctx).Create(snap).Error; err != nil {
        return nil, err
    }
    return result, nil
}
```

Bug #8 fix — aggregator adapters: Use `Raw()` with `Scan()` instead of `Model()` + aliased `Select()`

- [ ] **Step: Commit after each file**

### Task 3.7: Migrate remaining repos (delivery, annotation, theme, hitscorer, exporter)

**Files:**
- Create: `internal/repository/gormimpl/delivery.go`
- Create: `internal/repository/gormimpl/annotation.go`
- Create: `internal/repository/gormimpl/theme.go`
- Create: `internal/repository/gormimpl/hitscorer.go`
- Create: `internal/repository/gormimpl/exporter.go`

- [ ] **Step: Commit**

---

## Phase 4: Fx DI Assembly

### Task 4.1: Create Fx Module definitions

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

- [ ] **Step: Create infra module (example)**

```go
// internal/module/infra.go
package module

import (
    "go.uber.org/fx"
    "gorm.io/gorm"
    "github.com/redis/go-redis/v9"
)

var Infra = fx.Module("infra",
    fx.Provide(NewConfig),
    fx.Provide(NewGORMDB),
    fx.Provide(NewRedisClient),
    fx.Provide(NewLogger),
)

func NewGORMDB(cfg config.Config) (*gorm.DB, error) {
    return database.Open(cfg.DatabaseURL)
}

func NewRedisClient(cfg config.Config) *redis.Client {
    return redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
}
```

- [ ] **Step: Create business modules**

```go
// internal/module/monitor.go
package module

import (
    "go.uber.org/fx"
    "github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
    "github.com/StephenQiu30/hotkey-server/internal/service"
    "github.com/StephenQiu30/hotkey-server/internal/handler"
)

var MonitorModule = fx.Module("monitor",
    fx.Provide(gormimpl.NewMonitorRepo),
    fx.Provide(gormimpl.NewContentRepo),
    fx.Provide(monitor.NewService),
    fx.Provide(monitor.NewHandler),
)
```

- [ ] **Step: Commit**

```bash
git add internal/module/
git commit -m "feat: add Fx module definitions"
```

### Task 4.2: Create fxapp assembly point

**Files:**
- Create: `internal/fxapp/app.go`
- Modify: `cmd/hotkey/main.go` (simplify to fx.App().Run())

- [ ] **Step: Create fxapp/app.go**

```go
// internal/fxapp/app.go
package fxapp

import (
    "go.uber.org/fx"
    "github.com/StephenQiu30/hotkey-server/internal/module"
)

func NewApp() *fx.App {
    return fx.New(
        module.Infra,
        module.AuthModule,
        module.MonitorModule,
        module.TopicModule,
        module.EventModule,
        module.HotEventModule,
        module.DigestModule,
        module.TrendModule,
        module.NotifyModule,
        module.SchedulerModule,
    )
}
```

- [ ] **Step: Simplify main.go**

```go
// cmd/hotkey/main.go
package main

import "github.com/StephenQiu30/hotkey-server/internal/fxapp"

func main() {
    fxapp.NewApp().Run()
}
```

- [ ] **Step: Verify build + test start**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
make build
```

- [ ] **Step: Commit**

```bash
git add internal/fxapp/app.go cmd/hotkey/main.go
git commit -m "feat: wire Fx DI container as app entry point"
```

### Task 4.3: Clean up old code — delete obsolete files

**Files:**
- Delete: `internal/app/run.go`
- Delete: `internal/app/api_server.go`
- Delete: `internal/database/*repo.go`
- Delete: `internal/database/*query.go`
- Delete: `internal/database/array.go`
- Delete: `internal/database/models.go`
- Delete: `internal/database/hitscorer.go`
- Delete: `internal/database/exporter.go`
- Delete: `internal/database/worker_poll.go`
- Delete: `internal/database/worker_query.go`

- [ ] **Step: One-by-one delete and rebuild**

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
# Delete one file
rm internal/database/hoteventrepo.go
make build  # should still compile (migrated to gormimpl)
```

- [ ] **Step: Commit**

```bash
git rm internal/database/hoteventrepo.go
git commit -m "refactor: remove migrated hoteventrepo.go"
# Repeat for all deleted files
```

---

## Phase 5: Redis Cache Layer

### Task 5.1: Implement generic cache

**Files:**
- Create: `internal/cache/cache.go`

- [ ] **Step: Implement Cache[T] generic**

```go
// internal/cache/cache.go
package cache

import (
    "context"
    "encoding/json"
    "errors"
    "time"
    "github.com/redis/go-redis/v9"
)

type Cache[T any] struct {
    client *redis.Client
    prefix string
    ttl    time.Duration
}

func NewCache[T any](client *redis.Client, prefix string, ttl time.Duration) *Cache[T] {
    return &Cache[T]{client: client, prefix: prefix, ttl: ttl}
}

func (c *Cache[T]) key(id string) string { return c.prefix + ":" + id }

func (c *Cache[T]) Get(ctx context.Context, id string) (T, bool, error) {
    val, err := c.client.Get(ctx, c.key(id)).Result()
    if errors.Is(err, redis.Nil) {
        var zero T
        return zero, false, nil
    }
    if err != nil {
        var zero T
        return zero, false, err
    }
    var result T
    if err := json.Unmarshal([]byte(val), &result); err != nil {
        var zero T
        return zero, false, err
    }
    return result, true, nil
}

func (c *Cache[T]) Set(ctx context.Context, id string, val T) error {
    data, _ := json.Marshal(val)
    return c.client.Set(ctx, c.key(id), data, c.ttl).Err()
}

func (c *Cache[T]) Del(ctx context.Context, id string) error {
    return c.client.Del(ctx, c.key(id)).Err()
}
```

- [ ] **Step: Commit**

```bash
git add internal/cache/cache.go
git commit -m "feat: add generic Redis cache layer"
```

### Task 5.2: Implement hot event + topic cache

**Files:**
- Create: `internal/cache/hot_event_cache.go`
- Create: `internal/cache/topic_cache.go`

- [ ] **Step: Commit**

---

## Phase 6: Handler + Router Cleanup

### Task 6.1: Migrate handlers to use service interfaces via Fx

**Bug fixes embedded:** Bug #7 (log GetPlatforms error), Bug #10 (remove AppError nil guard)

**Files:**
- Modify: existing handler files to accept service interfaces via Fx injection
- Modify: `internal/middleware/` to be Fx-compatible

- [ ] **Step: One handler at a time**
- [ ] **Step: Commit**

### Task 6.2: Create unified router registration

**Files:**
- Create: `internal/router/router.go`

- [ ] **Step: Commit**

---

## Phase 7: Worker Lifecycle via Fx

### Task 7.1: Migrate workers to Fx lifecycle

**Bug fixes embedded:** Bug #20 (fix double signal.Notify)

**Files:**
- Create: `internal/worker/poll_worker.go`
- Create: `internal/worker/digest_worker.go`
- Create: `internal/worker/cleanup_worker.go`
- Create: `internal/worker/snapshot_worker.go`

- [ ] **Step: Commit**

---

## Phase 8: Testing Infrastructure

### Task 8.1: Generate mock code

```bash
cd /Users/stephenqiu/Desktop/StephenQiu30/HotKey/hotkey-server
mockgen -source=internal/repository/monitor.go -destination=internal/repository/mock/monitor.go
mockgen -source=internal/repository/user.go -destination=internal/repository/mock/user.go
# ... all repositories
```

### Task 8.2: Service layer unit tests (example — monitor)

```go
// internal/service/monitor_test.go
func TestMonitorService_Create(t *testing.T) {
    ctrl := gomock.NewController(t)
    mockRepo := mock_repository.NewMockMonitorRepository(ctrl)
    svc := monitor.NewService(mockRepo)
    
    mockRepo.EXPECT().
        Create(gomock.Any(), int64(1), gomock.Any()).
        Return(expectedMonitor, nil)
    
    result, err := svc.Create(context.Background(), 1, input)
    assert.NoError(t, err)
    assert.Equal(t, expectedMonitor, result)
}
```

### Task 8.3: Integration test setup with testcontainers

### Task 8.4: Update Makefile

```makefile
test-unit:      # go test ./internal/service/... ./internal/handler/... -v -count=1
test-integration: # go test ./tests/integration/... -v -count=1 -tags=integration
test-all:       # make test-unit && make test-integration
```

---

## Verification Gates

After each Phase, run:

```bash
make build
make test
```

## Bug Fix Verification Checklist

| # | Bug | Where Verified | Verification Method |
|---|-----|---------------|-------------------|
| 1 | Trend not persisted | Task 3.6 | Unit test: BuildTopicSnapshot + assert DB row exists |
| 2 | Wrong table in scoring | Task 3.4 | Integration test: poll flow, check monitor_post_hits has correct id |
| 3 | No context in HotEventRepo | Task 3.1 | Compiler: interface requires ctx parameter |
| 4 | No context in other repos | Task 3.2-3.7 | Compiler: interface requires ctx parameter |
| 5 | Silent JSON errors | Task 1.1 | JSONB[T] Scanner returns errors |
| 6 | Missing fields in UpsertHit | Task 3.4 | Code review: all fields set in UpsertHit |
| 7 | Silent error in trending handler | Task 6.1 | Code review: error logged |
| 8 | Model+Select aliasing | Task 3.6 | Code review: uses Raw+Scan instead |
| 9 | Cross-platform heat=0 | Task 3.6 | Code review: dedicated heat column mapping |
| 10 | AppError nil guard | Task 6.1 | Code review: guard removed |
| 11 | AuthRepo fragile zero check | Task 3.2 | Uses First()+ErrRecordNotFound |
| 12 | BuildSnapshots no ctx | Task 3.6 | Interface requires ctx |
| 13 | Duplicate delivery retry | Task 3.2 | Transactional approach |
| 14 | Ignored AddPlatform errors | Task 3.1 | Error propagated |
| 15-20 | Various LOW | Throughout | Code review confirmed |
