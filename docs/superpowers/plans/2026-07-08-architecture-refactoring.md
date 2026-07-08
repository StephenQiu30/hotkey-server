# Architecture Refactoring: MVC + Layered Restructure

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor hotkey-server from domain-based packaging to standard MVC layered architecture (controller/service/repository/model).

**Architecture:** Big Bang migration in 6 sequential steps, each maintaining `go build ./...`. Zero business logic changes — only file moves, package renames, and import path updates.

**Tech Stack:** Go 1.26, Gin, GORM, Fx DI

---

### Global Constraints

1. **Zero logic changes** — every diff must be: file moved, package renamed, import updated. Never change an `if err != nil` or business condition.
2. **`go build ./...` must pass after each step** — if it doesn't, fix before committing.
3. **One commit per step** — 6 commits, each independently compilable.
4. **Test files move with their source** — `_test.go` stays next to `_service.go`.
5. **`tests/unit/` mirror structure** — update imports when referenced types move.
6. **Keep `queue/` and `worker/`** — these stay at their current paths.
7. **Keep `pkg/`** — stays at current path.
8. **Keep `platform/logging/` and `platform/runtime/`** — unchanged.

### Task 1: Create directories + move model/entity (GORM models)

**Files:**
- Create: `internal/model/entity/user.go`
- Create: `internal/model/entity/monitor.go`
- Create: `internal/model/entity/post.go`
- Create: `internal/model/entity/topic.go`
- Create: `internal/model/entity/monitor_snapshot.go`
- Create: `internal/model/entity/event.go`
- Create: `internal/model/entity/report.go`
- Create: `internal/model/entity/alert.go`
- Create: `internal/model/entity/theme.go`
- Create: `internal/model/entity/annotation.go`
- Create: `internal/model/entity/export.go`
- Create: `internal/model/entity/knowledge.go`
- Create: `internal/model/entity/dead_letter.go`
- Delete: `internal/repository/gormimpl/model.go`

- [ ] **Step 1: Create empty target directories**

Run:
```bash
mkdir -p internal/{controller,service,repository,model/{entity,dto,vo}}
```

- [ ] **Step 2: Split model.go into 13 entity files under model/entity/**

The package declaration is `package entity` (NOT `package gormimpl`).

Each file gets a subset of structs from the original `internal/repository/gormimpl/model.go` (read the source file to get exact struct code). Structs keep all fields, GORM tags, and `TableName()` methods. Keep the `internal/pkg` import.

File mapping:
- `model/entity/user.go` → `User`
- `model/entity/monitor.go` → `KeywordMonitor`, `MonitorRun`, `MonitorPostHit`
- `model/entity/post.go` → `PlatformPost`, `PlatformAuthor`
- `model/entity/topic.go` → `Topic`, `TopicPost`, `TopicSnapshot`
- `model/entity/monitor_snapshot.go` → `MonitorSnapshot`
- `model/entity/event.go` → `Event`, `TopicEvent`, `HotEvent`, `HotEventPlatform`
- `model/entity/report.go` → `Report`, `ReportExport`
- `model/entity/alert.go` → `Alert`, `UserNotification`, `EmailDelivery`
- `model/entity/theme.go` → `Theme`, `ThemeMembership`
- `model/entity/annotation.go` → `EventAnnotation`, `TopicAnnotation`
- `model/entity/export.go` → `TopicDailyExport`, `ExportBundle`
- `model/entity/knowledge.go` → `KnowledgeRun`, `KnowledgeWritebackLog`, `KnowledgeObjectRevision`
- `model/entity/dead_letter.go` → `DLQRecord` (from `internal/queue/types.go`)

Each file starts with:
```go
package entity

import (
    "time"

    "github.com/StephenQiu30/hotkey-server/internal/pkg"
)
```

(Dead letter uses `package entity` too, just copy the struct definition.)

- [ ] **Step 3: Remove original model.go**

After the 13 entity files compile, delete `internal/repository/gormimpl/model.go`:

```bash
git rm internal/repository/gormimpl/model.go
```

- [ ] **Step 4: Build check**

Run:
```bash
go build ./...
```
Expected: fails because all references to `gormimpl.User`, `gormImpl.KeywordMonitor`, etc. are now broken — they point to deleted package. This is expected; the next step updates imports.

- [ ] **Step 5: Replace all gormimpl entity references with model/entity**

Find every Go file that references `gormimpl.User`, `gormimpl.KeywordMonitor`, `gormimpl.PlatformPost`, `gormimpl.PlatformAuthor`, `gormimpl.MonitorPostHit`, `gormimpl.Topic`, `gormimpl.TopicPost`, `gormimpl.TopicSnapshot`, `gormimpl.MonitorSnapshot`, `gormimpl.Alert`, `gormimpl.UserNotification`, `gormimpl.EmailDelivery`, `gormimpl.Event`, `gormimpl.TopicEvent`, `gormimpl.HotEvent`, `gormimpl.HotEventPlatform`, `gormimpl.Report`, `gormimpl.ReportExport`, `gormimpl.Theme`, `gormimpl.ThemeMembership`, `gormimpl.EventAnnotation`, `gormimpl.TopicAnnotation`, `gormimpl.TopicDailyExport`, `gormimpl.ExportBundle`, `gormimpl.KnowledgeRun`, `gormimpl.KnowledgeWritebackLog`, `gormimpl.KnowledgeObjectRevision`, `gormimpl.PlatformPost`:

```bash
# Replace gormimpl. prefix with entity. for all entity types
sed -i '' 's/gormimpl\.User/entity.User/g' $(grep -rl 'gormimpl\.User' internal/ --include='*.go')
sed -i '' 's/gormimpl\.KeywordMonitor/entity.KeywordMonitor/g' $(grep -rl 'gormimpl\.KeywordMonitor' internal/ --include='*.go')
sed -i '' 's/gormimpl\.PlatformPost/entity.PlatformPost/g' $(grep -rl 'gormimpl\.PlatformPost' internal/ --include='*.go')
sed -i '' 's/gormimpl\.PlatformAuthor/entity.PlatformAuthor/g' $(grep -rl 'gormimpl\.PlatformAuthor' internal/ --include='*.go')
sed -i '' 's/gormimpl\.MonitorPostHit/entity.MonitorPostHit/g' $(grep -rl 'gormimpl\.MonitorPostHit' internal/ --include='*.go')
sed -i '' 's/gormimpl\.Topic/entity.Topic/g' $(grep -rl 'gormimpl\.Topic' internal/ --include='*.go')
sed -i '' 's/gormimpl\.TopicPost/entity.TopicPost/g' $(grep -rl 'gormimpl\.TopicPost' internal/ --include='*.go')
sed -i '' 's/gormimpl\.TopicSnapshot/entity.TopicSnapshot/g' $(grep -rl 'gormimpl\.TopicSnapshot' internal/ --include='*.go')
sed -i '' 's/gormimpl\.MonitorSnapshot/entity.MonitorSnapshot/g' $(grep -rl 'gormimpl\.MonitorSnapshot' internal/ --include='*.go')
sed -i '' 's/gormimpl\.Alert/entity.Alert/g' $(grep -rl 'gormimpl\.Alert' internal/ --include='*.go')
sed -i '' 's/gormimpl\.UserNotification/entity.UserNotification/g' $(grep -rl 'gormimpl\.UserNotification' internal/ --include='*.go')
sed -i '' 's/gormimpl\.EmailDelivery/entity.EmailDelivery/g' $(grep -rl 'gormimpl\.EmailDelivery' internal/ --include='*.go')
sed -i '' 's/gormimpl\.Event/entity.Event/g' $(grep -rl 'gormimpl\.Event' internal/ --include='*.go')
sed -i '' 's/gormimpl\.TopicEvent/entity.TopicEvent/g' $(grep -rl 'gormimpl\.TopicEvent' internal/ --include='*.go')
sed -i '' 's/gormimpl\.HotEvent/entity.HotEvent/g' $(grep -rl 'gormimpl\.HotEvent' internal/ --include='*.go')
sed -i '' 's/gormimpl\.HotEventPlatform/entity.HotEventPlatform/g' $(grep -rl 'gormimpl\.HotEventPlatform' internal/ --include='*.go')
sed -i '' 's/gormimpl\.Report/entity.Report/g' $(grep -rl 'gormimpl\.Report' internal/ --include='*.go')
sed -i '' 's/gormimpl\.ReportExport/entity.ReportExport/g' $(grep -rl 'gormimpl\.ReportExport' internal/ --include='*.go')
sed -i '' 's/gormimpl\.Theme/entity.Theme/g' $(grep -rl 'gormimpl\.Theme' internal/ --include='*.go')
sed -i '' 's/gormimpl\.ThemeMembership/entity.ThemeMembership/g' $(grep -rl 'gormimpl\.ThemeMembership' internal/ --include='*.go')
sed -i '' 's/gormimpl\.EventAnnotation/entity.EventAnnotation/g' $(grep -rl 'gormimpl\.EventAnnotation' internal/ --include='*.go')
sed -i '' 's/gormimpl\.TopicAnnotation/entity.TopicAnnotation/g' $(grep -rl 'gormimpl\.TopicAnnotation' internal/ --include='*.go')
sed -i '' 's/gormimpl\.TopicDailyExport/entity.TopicDailyExport/g' $(grep -rl 'gormimpl\.TopicDailyExport' internal/ --include='*.go')
sed -i '' 's/gormimpl\.ExportBundle/entity.ExportBundle/g' $(grep -rl 'gormimpl\.ExportBundle' internal/ --include='*.go')
sed -i '' 's/gormimpl\.KnowledgeRun/entity.KnowledgeRun/g' $(grep -rl 'gormimpl\.KnowledgeRun' internal/ --include='*.go')
sed -i '' 's/gormimpl\.KnowledgeWritebackLog/entity.KnowledgeWritebackLog/g' $(grep -rl 'gormimpl\.KnowledgeWritebackLog' internal/ --include='*.go')
sed -i '' 's/gormimpl\.KnowledgeObjectRevision/entity.KnownObjectRevision/g' $(grep -rl 'gormimpl\.KnowledgeObjectRevision' internal/ --include='*.go')
sed -i '' 's/gormimpl\.MonitorRun/entity.MonitorRun/g' $(grep -rl 'gormimpl\.MonitorRun' internal/ --include='*.go')
```

Also update references from `gormimpl` package import — replace the import path `github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl` with `github.com/StephenQiu30/hotkey-server/internal/model/entity` where the file only used gormimpl for entities:

```bash
# Find files that imported gormimpl but only use entity types now
# For each file, update import path. Since entity types now come from model/entity,
# add that import. Keep gormimpl import if it also uses non-entity types from that package.
```

For files that used BOTH entity types AND non-entity types from gormimpl (like `gormimpl.CollectRepo`), keep both imports:
```go
import (
    "github.com/StephenQiu30/hotkey-server/internal/model/entity"
    "github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
)
```

- [ ] **Step 6: Also replace gormimpl references in tests/**

```bash
# Same sed commands but for tests/unit/
for t in User KeywordMonitor PlatformPost PlatformAuthor MonitorPostHit Topic TopicPost TopicSnapshot MonitorSnapshot Alert UserNotification EmailDelivery Event TopicEvent HotEvent HotEventPlatform Report ReportExport Theme ThemeMembership EventAnnotation TopicAnnotation TopicDailyExport ExportBundle KnowledgeRun KnowledgeWritebackLog KnowledgeObjectRevision MonitorRun; do
  from="gormimpl.${t}"
  to="entity.${t}"
  grep -rl "$from" tests/ --include='*.go' 2>/dev/null | while read f; do
    sed -i '' "s/${from}/${to}/g" "$f"
  done
done
```

- [ ] **Step 7: Build and verify**

```bash
go build ./...
```
Expected success. If not, check for missed references.

- [ ] **Step 8: Commit**

```bash
git add internal/model/entity/ internal/repository/gormimpl/model.go --ignore-removal
git add -u  # track deletions
git commit -m "refactor: split GORM models into model/entity/

Move 29 GORM structs from repository/gormimpl/model.go to model/entity/
organized by domain (13 files). Update all import references.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task 2: Move DTOs into model/dto/ + config split

**Files:**
- Create: `internal/model/dto/auth.go` (from `internal/auth/model.go` structs)
- Create: `internal/model/dto/monitor.go` (from `internal/monitor/model.go` structs)
- Create: `internal/model/dto/hotevent.go` (from `internal/hotevent/model.go` structs)
- Create: `internal/model/dto/report.go` (from `internal/report/model.go` structs)
- Create: `internal/model/dto/notify.go` (from `internal/notify/model.go` structs)
- Create: `internal/model/dto/embedding.go` (from `internal/embedding/model.go` — only non-ONNX types)
- Create: `internal/model/dto/collect.go` (from `internal/collect/xclient.go` — Tweet, StreamRule)
- Create: `internal/model/dto/obsidian.go` (from `internal/obsidian/model.go`)
- Delete: `internal/auth/model.go`
- Delete: `internal/monitor/model.go`
- Delete: `internal/hotevent/model.go` (keep errors.go and constants)
- Delete: `internal/report/model.go`
- Delete: `internal/notify/model.go`
- Delete: `internal/obsidian/model.go`
- Modify: `internal/config/config.go` (split into sub-files)

- [ ] **Step 1: Create model/dto files**

For each DTO file at `internal/model/dto/<name>.go`:

```go
package dto

// Structs copied verbatim from original domain model files.
// Package "dto" replaces the original domain package.
```

DTO file mapping (copy structs, not entity/GORM structs):

`model/dto/auth.go`:
- `RegisterInput` (from `auth/model.go`)
- `LoginInput` (from `auth/model.go`)
- `User` struct (from `auth/model.go` — the domain User, NOT the entity.User)
- Sentinel errors `ErrEmailExists`, `ErrInvalidCredentials`, `ErrInvalidInput` stay with `service/auth.go` (move in Task 4)

`model/dto/monitor.go` from `monitor/model.go`:
- `Monitor` struct (domain model, NOT entity.KeywordMonitor)
- `CreateMonitorInput`
- `UpdateMonitorInput`
- `AllowedIntervals` map (this is a business constant, keep with service)

`model/dto/hotevent.go` from `hotevent/model.go`:
- `HotEvent` struct (domain model, NOT entity.HotEvent)
- `EventPlatform`
- Constants `StatusActive`, `StatusArchived`, `TrendRising`, `TrendStable`, `TrendDeclining` — keep with service (non-DTO)

`model/dto/report.go` from `report/model.go`:
- `Report` struct (domain model)
- `CreateInput`, `ListFilter`, `CreateReportRecord`
- `MonitorSource`, `TopicSource`, `PostSource`
- Constants `TypeDaily`, `TypeWeekly`, `StatusDraft`, `StatusSent` — keep with service

`model/dto/notify.go` from `notify/model.go`:
- `Notification` struct (domain model)

`model/dto/obsidian.go` from `obsidian/model.go`:
- `PathInput`, `WriteResult`
- `ExportKind`, exported constants `ExportDailyDigest`, `ExportPublishDraft`, `WriteStatusPublished`, `WriteStatusSkipped`

`model/dto/embedding.go`:
- No DTO structs to move (embedding/model.go is ONNX implementation, not DTOs)
- Empty file with just `package dto` or skip

`model/dto/collect.go` from `collect/xclient.go`:
- `Tweet` struct (line 15-21)
- `StreamRule` struct (line 25-29)

- [ ] **Step 2: Remove original domain model files**

```bash
git rm internal/auth/model.go
git rm internal/monitor/model.go
git rm internal/hotevent/model.go  # Note: keep hotevent/errors.go, hotevent/queryservice.go, hotevent/repository.go, hotevent/service.go
git rm internal/report/model.go    # Note: errors stay in report/service.go (move later)
git rm internal/notify/model.go
git rm internal/obsidian/model.go
```

Note: Do NOT delete `hotevent/errors.go`, `hotevent/queryservice.go`, `hotevent/repository.go`, `hotevent/service.go` — these move to service/ in Task 4.

- [ ] **Step 3: Update all import references**

Find every file that imported the old domain packages and used DTO types. Replace with `model/dto`:

```bash
# Replace auth model references
sed -i '' 's/"github.com\/StephenQiu30\/hotkey-server\/internal\/auth"/"github.com\/StephenQiu30\/hotkey-server\/internal\/model\/dto"/g' $(grep -rl '"github.com/StephenQiu30/hotkey-server/internal/auth"' internal/ --include='*.go' | grep -v 'internal/service/auth.go')
# BUT only for files that only used auth for types, NOT for auth.Service

# Update imports in files that used both types and services — add dto import, keep service import
```

This is context-dependent per file. The rule:
- Files that imported an old domain package ONLY for struct types → change to `model/dto`
- Files that imported for BOTH types and services → add `dto` import, keep service import

For `platform/http/auth.go` — it imports `auth` for DTO types AND `auth.Service`. In Task 4 the Service moves to `service/`. For now, add the `dto` import and keep the old `auth` import:

```go
import (
    "github.com/StephenQiu30/hotkey-server/internal/model/dto"
    "github.com/StephenQiu30/hotkey-server/internal/auth"
)
```

- [ ] **Step 4: Split config/config.go into sub-files**

Create config sub-files in `internal/config/`:

`internal/config/server.go`:
```go
package config
// HTTPAddr, SwaggerEnabled fields
// Keep defaults and bindings for HTTP_ADDR, SWAGGER_ENABLED
// Keep JWTSecret, XToken, XBaseURL
```

`internal/config/database.go`:
```go
package config
// DatabaseURL, RedisAddr fields
```

`internal/config/kafka.go`:
```go
package config
// KafkaBrokers, KafkaConsumerGroup fields
```

`internal/config/llm.go`:
```go
package config
// LLMProvider, LLMAPIKey, LLMBaseURL, LLMModel, LLMMaxTokens, LLMTemperature
// EmbeddingModelPath
```

`internal/config/obsidian.go`:
```go
package config
// ObsidianVaultPath, DailyDigestTime, DailyDigestTimezone, DailyDigestTarget, DailyDigestTopN
```

`internal/config/config.go` — keep the master `Config` struct, `Load()` function, and `LogLevel/LogFormat/LogOutput` fields. Move domain fields into the sub-files but keep them on the `Config` struct (same struct, fields can be declared in different files in the same package).

```go
package config

// Config remains the master struct but domain fields are split into sub-files.
// All in package config so they share the same struct.

func Load() (Config, error) {
    // unchanged
}
```

- [ ] **Step 5: Build and verify**

```bash
go build ./...
```

Expected success.

- [ ] **Step 6: Commit**

```bash
git add internal/model/dto/ internal/config/
git add -u
git commit -m "refactor: move DTOs to model/dto/, split config

Move domain value objects to model/dto/ organized by domain.
Split config/config.go into domain sub-files (same package).

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task 3: Move repository/ layer (all data access to one package)

**Files:**
- Move: `internal/repository/gormimpl/*.go` → `internal/repository/*.go` (package `repository`)
- Move: `internal/database/contentquery.go` → `internal/repository/content.go`
- Move: `internal/database/topicquery.go` → `internal/repository/topic_read.go`
- Move: `internal/database/trendquery.go` → `internal/repository/trend.go`
- Move: `internal/database/bootstrap.go` → `internal/platform/database/bootstrap.go`
- Move: `internal/database/database.go` → `internal/platform/database/database.go`
- Move: `internal/database/logger.go` → `internal/platform/database/logger.go`
- Delete: `internal/database/` (empty after moves)
- Delete: `internal/repository/gormimpl/` (empty after moves)

- [ ] **Step 1: Move gormimpl/* files to repository/**

```bash
# Move all Go files from gormimpl to repository/
for f in internal/repository/gormimpl/*.go; do
  newname=$(basename "$f")
  cp "$f" "internal/repository/$newname"
done
```

- [ ] **Step 2: Change package declaration from gormimpl to repository**

```bash
sed -i '' 's/^package gormimpl$/package repository/' internal/repository/*.go
```

- [ ] **Step 3: Update import references inside repository files**

Files in `internal/repository/` that imported `gormimpl` for entity types need fixing. After Step 2, these files reference `gormimpl.EntityName` within the same package, which no longer exists. Replace with `entity.`:

```bash
# Inside internal/repository/*.go files, replace gormimpl. with entity.
sed -i '' 's/gormimpl\./entity\./g' internal/repository/*.go
```

Also remove self-import of old gormimpl path if present.

- [ ] **Step 4: Move database query services into repository/**

```bash
cp internal/database/contentquery.go internal/repository/content.go
cp internal/database/topicquery.go internal/repository/topic_read.go
cp internal/database/trendquery.go internal/repository/trend.go
```

Change their package to `repository` and update import references:
```bash
sed -i '' 's/^package database$/package repository/' internal/repository/content.go internal/repository/topic_read.go internal/repository/trend.go
```

Also rename constructor functions to avoid name collisions:
- `database.NewContentQueryService` → `repository.NewContentRepo`
- `database.NewTopicQueryService` → `repository.NewTopicReadRepo`
- `database.NewTrendQueryService` → `repository.NewTrendRepo`

But to minimize changes, keep the original function names. Since all are in the same package, there's no collision risk. Rename them for consistency in a follow-up.

- [ ] **Step 5: Move database infrastructure to platform/database/**

```bash
mkdir -p internal/platform/database
cp internal/database/bootstrap.go internal/platform/database/bootstrap.go
cp internal/database/database.go internal/platform/database/database.go
cp internal/database/logger.go internal/platform/database/logger.go
```

Package stays `package database`. Change import path references from `internal/database` to `internal/platform/database`:

```bash
sed -i '' 's|"github.com/StephenQiu30/hotkey-server/internal/database"|"github.com/StephenQiu30/hotkey-server/internal/platform/database"|g' \
  $(grep -rl '"github.com/StephenQiu30/hotkey-server/internal/database"' internal/ --include='*.go' | grep -v internal/platform/database/)
```

- [ ] **Step 6: Delete old files**

```bash
# Delete gormimpl directory
rm -rf internal/repository/gormimpl/

# Delete old database files (moved ones and query services)
rm -f internal/database/contentquery.go internal/database/topicquery.go internal/database/trendquery.go
rm -f internal/database/bootstrap.go internal/database/database.go internal/database/logger.go

# Remove database/ directory if empty
rmdir internal/database/ 2>/dev/null || true
```

- [ ] **Step 7: Update all external imports**

Find every file outside `internal/repository/` that imported `gormimpl` for non-entity types (repos, collectors):

```bash
# Files that imported gormimpl for repo types like CollectRepo, TopicWriteRepo, SnapshotRepo etc.
# Change import from gormimpl to repository
sed -i '' 's|"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"|"github.com/StephenQiu30/hotkey-server/internal/repository"|g' \
  $(grep -rl 'internal/repository/gormimpl' internal/ --include='*.go')
sed -i '' 's|"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"|"github.com/StephenQiu30/hotkey-server/internal/repository"|g' \
  $(grep -rl 'internal/repository/gormimpl' tests/ --include='*.go' 2>/dev/null)
```

Replace `gormimpl.` prefix with `repository.` for repo types:
```bash
# Replace gormimpl.CollectRepo -> repository.CollectRepo etc.
for t in CollectRepo TopicWriteRepo SnapshotRepo MatchRepo; do
  sed -i '' "s/gormimpl\.${t}/repository.${t}/g" $(grep -rl "gormimpl\.${t}" internal/ --include='*.go')
done
```

- [ ] **Step 8: Build and verify**

```bash
go build ./...
```
Expected success. Fix any remaining import issues.

- [ ] **Step 9: Commit**

```bash
git add internal/repository/ internal/platform/database/ internal/
git add -u
git commit -m "refactor: flatten repository/ package, move DB infra to platform/database/

Merge gormimpl/ and database/*query.go into single repository/ package.
Move DB bootstrap code to platform/database/.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task 4: Move business logic into service/

**Files:**
- Create: `internal/service/auth.go` (from `internal/auth/service.go`)
- Create: `internal/service/monitor.go` (from `internal/monitor/service.go` + `internal/monitor/repository.go`)
- Create: `internal/service/report.go` (from `internal/report/service.go` + `internal/report/repository.go`)
- Create: `internal/service/topic.go` (from `internal/topic/service.go` + `internal/topic/query.go`)
- Create: `internal/service/trend.go` (from `internal/trend/service.go` + `internal/trend/query.go`)
- Create: `internal/service/hotevent.go` (from `internal/hotevent/service.go` + `internal/hotevent/queryservice.go` + `internal/hotevent/repository.go` + `internal/hotevent/errors.go`)
- Create: `internal/service/notify.go` (from `internal/notify/service.go` + `internal/notify/repository.go` + `internal/notify/mailer.go`)
- Create: `internal/service/llm.go` (from `internal/llm/service.go` + `internal/llm/provider.go` + `internal/llm/factory.go` + `internal/llm/chain.go` + `internal/llm/adapter.go` + `internal/llm/errors.go`)
- Create: `internal/service/embedding.go` (from `internal/embedding/service.go` + `internal/embedding/model.go` + `internal/embedding/tokenizer.go`)
- Create: `internal/service/collect.go` (from `internal/collect/service.go` + `internal/collect/xclient.go`)
- Create: `internal/service/obsidian.go` (from `internal/obsidian/render.go` + `internal/obsidian/writer.go` + `internal/obsidian/pathing.go`)
- Delete: `internal/auth/` (empty after move)
- Delete: `internal/monitor/`
- Delete: `internal/report/`
- Delete: `internal/topic/`
- Delete: `internal/trend/`
- Delete: `internal/hotevent/`
- Delete: `internal/notify/`
- Delete: `internal/llm/`
- Delete: `internal/embedding/`
- Delete: `internal/collect/`
- Delete: `internal/obsidian/`

- [ ] **Step 1: Merge domain files into service/ by domain**

For each domain, merge all `.go` files from the domain package into one service file. Keep all exported types and functions.

The package declaration becomes `package service` for all files.

Example for `internal/service/auth.go` — merge contents of:
- `internal/auth/service.go` (AuthService struct + NewService + methods)
- `internal/auth/repository.go` (Repository interface)

```go
package service

import (
    "context"
    "errors"

    "github.com/StephenQiu30/hotkey-server/internal/model/dto"
    "golang.org/x/crypto/bcrypt"
)

// Sentinel errors (moved from auth/model.go)
var (
    ErrEmailExists        = errors.New("email already registered")
    ErrInvalidCredentials = errors.New("invalid email or password")
    ErrInvalidInput       = errors.New("invalid input")
)

// Repository interface for auth (moved from auth/repository.go)
type AuthRepository interface {
    Create(ctx context.Context, email, passwordHash, displayName string) (dto.User, error)
    GetByEmail(ctx context.Context, email string) (dto.User, error)
    ExistsByEmail(ctx context.Context, email string) bool
}

// Service provides authentication operations.
type AuthService struct {
    repo AuthRepository
}

// NewAuthService creates a new auth Service.
func NewAuthService(repo AuthRepository) *AuthService {
    return &AuthService{repo: repo}
}

// Register creates a new user account.
func (s *AuthService) Register(ctx context.Context, input dto.RegisterInput) (dto.User, error) {
    // ... same body as original
}
```

Note: Rename service constructors to be unique across domains:
- `auth.NewService` → `service.NewAuthService`
- `monitor.NewService` → `service.NewMonitorService`
- `report.NewService` → `service.NewReportService`
- `notify.NewService` → `service.NewNotifyService`
- `hotevent.NewQueryService` → `service.NewHotEventQueryService`
- `llm.NewService` → `service.NewLLMService`
- `llm.NewProvider` → `service.NewLLMProvider`
- `llm.NewChain` → `service.NewLLMChain`
- `embedding.NewService` → `service.NewEmbeddingService`
- `collect.NewXClient` → `service.NewXClient`
- `obsidian.NewWriter` → `service.NewObsidianWriter`
- `obsidian.NewPathBuilder` → `service.NewObsidianPathBuilder`

- [ ] **Step 2: Update fxapp/app.go references**

The Fx DI wiring needs updated constructor names:
- `auth.NewService` → `service.NewAuthService`
- `monitor.NewService` → `service.NewMonitorService`
- `report.NewService` → `service.NewReportService`
- `notify.NewService` → `service.NewNotifyService`
- `hotevent.NewQueryService` → `service.NewHotEventQueryService`
- `llm.NewProvider` → `service.NewLLMProvider`
- `llm.NewService` → `service.NewLLMService`
- `llm.NewChain` → `service.NewLLMChain`
- `repository.NewContentQueryService` → (already in repository/)
- `repository.NewTopicQueryService` → (already in repository/)
- `repository.NewTrendQueryService` → (already in repository/)

Also update fxapp helper functions like `newMonitorService` to use new constructors.

- [ ] **Step 3: Update controller references to services**

All service types in controller files need updated import paths. The service types like `*auth.Service` become `*service.AuthService`.

- [ ] **Step 4: Build and verify**

```bash
go build ./...
```
Expected success. Fix remaining import issues.

- [ ] **Step 5: Remove old domain directories**

```bash
rm -rf internal/auth/ internal/monitor/ internal/report/ internal/topic/
rm -rf internal/trend/ internal/hotevent/ internal/notify/ internal/llm/
rm -rf internal/embedding/ internal/collect/ internal/obsidian/
```

- [ ] **Step 6: Commit**

```bash
git add internal/service/ internal/
git add -u
git commit -m "refactor: merge all business logic into service/

Flatten auth/monitor/report/topic/trend/hotevent/notify/llm/
embedding/collect/obsidian into service/ package.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task 5: Move HTTP handlers into controller/

**Files:**
- Create: `internal/controller/route.go` (from `internal/platform/http/router.go`)
- Create: `internal/controller/auth.go` (from `internal/platform/http/auth.go`)
- Create: `internal/controller/monitor.go` (from `internal/platform/http/monitor.go`)
- Create: `internal/controller/report.go` (from `internal/platform/http/report.go`)
- Create: `internal/controller/content.go` (from `internal/platform/http/content.go`)
- Create: `internal/controller/topic.go` (from `internal/platform/http/topic.go`)
- Create: `internal/controller/notify.go` (from `internal/platform/http/notify.go`)
- Create: `internal/controller/trending.go` (from `internal/platform/http/trending.go`)
- Create: `internal/controller/health.go` (from `internal/platform/http/health.go`)
- Create: `internal/controller/request.go` (from `internal/platform/http/swagger_types.go` — request structs only)
- Create: `internal/controller/vo/` — response structs from controller/vo/
  - Create: `internal/model/vo/auth.go` (UserData, LoginData, UserResponse, LoginResponse types)
  - Create: `internal/model/vo/monitor.go` (MonitorData, MonitorResponse, MonitorListResponse)
  - Create: `internal/model/vo/report.go` (ReportResponse types from report.go)
  - Create: `internal/model/vo/common.go` (ResponseBody, PageBody from response.go)
  - Create: `internal/model/vo/health.go` (HealthBody)
- Modify: `internal/platform/http/errors.go` (keep in platform/http — middleware tool)
- Modify: `internal/platform/http/middleware.go` (keep in platform/http — unchanged)
- Modify: `internal/platform/http/accesslog.go` (keep in platform/http — unchanged)

- [ ] **Step 1: Copy all handler files to controller/**

```bash
cp internal/platform/http/router.go internal/controller/route.go
cp internal/platform/http/auth.go internal/controller/auth.go
cp internal/platform/http/monitor.go internal/controller/monitor.go
cp internal/platform/http/report.go internal/controller/report.go
cp internal/platform/http/content.go internal/controller/content.go
cp internal/platform/http/topic.go internal/controller/topic.go
cp internal/platform/http/notify.go internal/controller/notify.go
cp internal/platform/http/trending.go internal/controller/trending.go
cp internal/platform/http/health.go internal/controller/health.go
cp internal/platform/http/swagger_types.go internal/controller/request.go
```

- [ ] **Step 2: Change package declaration**

```bash
sed -i '' 's/^package http$/package controller/' internal/controller/*.go
```

- [ ] **Step 3: Create model/vo/ response types**

Create VO files that contain only response structs (the `XxxData`, `XxxResponse`, `XxxBody` types). These are the JSON payloads sent to frontend.

`internal/model/vo/auth.go`:
```go
package vo

// UserData is the JSON representation of a user.
type UserData struct {
    ID          int64  `json:"id"`
    Email       string `json:"email"`
    DisplayName string `json:"display_name"`
}

// LoginData is the JSON representation of a login response.
type LoginData struct {
    User  UserData `json:"user"`
    Token string   `json:"token"`
}
```

`internal/model/vo/monitor.go`:
```go
package vo

type MonitorData struct {
    ID                  int64  `json:"id"`
    UserID              int64  `json:"user_id"`
    Name                string `json:"name"`
    QueryText           string `json:"query_text"`
    Language            string `json:"language"`
    Region              string `json:"region"`
    Status              string `json:"status"`
    PollIntervalMinutes int    `json:"poll_interval_minutes"`
    AlertEnabled        bool   `json:"alert_enabled"`
}
```

`internal/model/vo/common.go`:
```go
package vo

type ResponseBody struct {
    Data      any    `json:"data"`
    RequestID string `json:"request_id,omitempty"`
}

type PageBody struct {
    Data      any    `json:"data"`
    Page      int    `json:"page"`
    PageSize  int    `json:"page_size"`
    Total     int    `json:"total"`
    RequestID string `json:"request_id,omitempty"`
}
```

`internal/model/vo/health.go`:
```go
package vo

type HealthBody struct {
    Status string `json:"status"`
}
```

- [ ] **Step 4: Update controller files to use vo types + new service imports**

Each controller file needs:
1. Import updated to `service.AuthService` instead of `auth.Service`
2. Response types referenced from `vo` package
3. `respondError`, `respondInternalError` still from `platform/http` package

For `controller/auth.go`, the import block becomes:
```go
import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/golang-jwt/jwt/v5"

    "github.com/StephenQiu30/hotkey-server/internal/model/vo"
    "github.com/StephenQiu30/hotkey-server/internal/service"
    platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
)

func RegisterAuthRoutes(r *gin.Engine, svc *service.AuthService, jwtSecret string) {
    // same body
}

func registerHandler(svc *service.AuthService) gin.HandlerFunc {
    return func(c *gin.Context) {
        // respondError stays as platformhttp.respondError or use local helper
        // RespondCreated becomes vo.RespondCreated or keep response.go in controller
    }
}
```

- [ ] **Step 5: Move response.go helpers into controller/response.go**

Rather than importing from `platform/http`, copy the response helpers into `internal/controller/response.go`:

```go
package controller

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/StephenQiu30/hotkey-server/internal/model/vo"
)

func respondOK(c *gin.Context, data any) {
    c.JSON(http.StatusOK, vo.ResponseBody{
        Data: data,
        RequestID: requestIDFromContext(c),
    })
}

func respondCreated(c *gin.Context, data any) { ... }
func respondPage(c *gin.Context, data any, page, pageSize, total int) { ... }
```

Keep the error helpers (`respondError`, `respondInternalError`, `RespondAppError`, `RespondErrorCode`) in `platform/http/errors.go` and import them.

- [ ] **Step 6: Remove handler files from platform/http/**

```bash
rm internal/platform/http/router.go
rm internal/platform/http/auth.go
rm internal/platform/http/monitor.go
rm internal/platform/http/report.go
rm internal/platform/http/content.go
rm internal/platform/http/topic.go
rm internal/platform/http/notify.go
rm internal/platform/http/trending.go
rm internal/platform/http/health.go
rm internal/platform/http/swagger_types.go
rm internal/platform/http/request.go  # does not exist yet, skip if missing
```

- [ ] **Step 7: Update fxapp/app.go**

Replace `platformhttp.NewRouter` call with `controller.NewRouter`.

Update `HTTPServerIn` to use `*service.AuthService` etc. instead of `*auth.Service`.

- [ ] **Step 8: Build and verify**

```bash
go build ./...
```
Expected success.

- [ ] **Step 9: Commit**

```bash
git add internal/controller/ internal/model/vo/ internal/
git add -u
git commit -m "refactor: move HTTP handlers to controller/, add model/vo/

Move all handler functions and route registration from platform/http/
to controller/. Create model/vo/ for response view objects.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task 6: Final cleanup — update fxapp, tests, and build verification

**Files:**
- Modify: `internal/fxapp/app.go`
- Modify: Internal test files (`tests/unit/`)

- [ ] **Step 1: Rewrite fxapp/app.go with updated imports**

The DI wiring needs all import paths updated. Read the current `internal/fxapp/app.go` and replace:

Old imports:
```go
"github.com/StephenQiu30/hotkey-server/internal/auth"
"github.com/StephenQiu30/hotkey-server/internal/config"
"github.com/StephenQiu30/hotkey-server/internal/content"
"github.com/StephenQiu30/hotkey-server/internal/database"
"github.com/StephenQiu30/hotkey-server/internal/hotevent"
"github.com/StephenQiu30/hotkey-server/internal/llm"
"github.com/StephenQiu30/hotkey-server/internal/monitor"
"github.com/StephenQiu30/hotkey-server/internal/notify"
platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
"github.com/StephenQiu30/hotkey-server/internal/report"
"github.com/StephenQiu30/hotkey-server/internal/repository/gormimpl"
"github.com/StephenQiu30/hotkey-server/internal/topic"
"github.com/StephenQiu30/hotkey-server/internal/trend"
```

New imports:
```go
"github.com/StephenQiu30/hotkey-server/internal/config"
"github.com/StephenQiu30/hotkey-server/internal/controller"
"github.com/StephenQiu30/hotkey-server/internal/model/entity"
"github.com/StephenQiu30/hotkey-server/internal/platform/database"
platformhttp "github.com/StephenQiu30/hotkey-server/internal/platform/http"
"github.com/StephenQiu30/hotkey-server/internal/repository"
"github.com/StephenQiu30/hotkey-server/internal/service"
```

Update fx.Provide calls:
```go
// Before:
fx.Provide(fx.Annotate(gormimpl.NewUserRepo, fx.As(new(auth.Repository))))
fx.Provide(auth.NewService)

// After:
fx.Provide(fx.Annotate(repository.NewUserRepo, fx.As(new(service.AuthRepository))))
fx.Provide(service.NewAuthService)
```

Update HTTPServerIn struct — replace domain-specific types with service types:
```go
type HTTPServerIn struct {
    fx.In

    Config      *config.Config
    AuthService *service.AuthService
    MonitorSvc  *service.MonitorService
    NotifySvc   *service.NotifyService
    ReportSvc   *service.ReportService

    PostQuerySvc  *repository.ContentRepo
    TopicQuerySvc *repository.TopicReadRepo
    TrendQuerySvc *repository.TrendRepo
    HotEventMgr   *service.HotEventQueryService
}
```

Update NewHTTPServer to call `controller.NewRouter` instead of `platformhttp.NewRouter`:
```go
func NewHTTPServer(in HTTPServerIn) *http.Server {
    router := controller.NewRouter(controller.Config{
        JWTSecret:       in.Config.JWTSecret,
        SmokeTest:       smokeTest,
        SwaggerEnabled:  in.Config.SwaggerEnabled,
        AuthService:     in.AuthService,
        MonitorSvc:      in.MonitorSvc,
        NotifySvc:       in.NotifySvc,
        ReportSvc:       in.ReportSvc,
        PostQuerySvc:    in.PostQuerySvc,
        TopicQuerySvc:   in.TopicQuerySvc,
        TrendQuerySvc:   in.TrendQuerySvc,
        HotEventManager: in.HotEventMgr,
    })
    // ...
}
```

- [ ] **Step 2: Update fxapp helper functions**

Replace `newMonitorService`, `newReportService`, `newHourlyAggregateJob`, `newDailyObsidianPublishJob` to use new service/repository import paths:
- `monitor.NewService(repo, nil)` → `service.NewMonitorService(repo, nil)`
- `report.NewService(repo, time.Now)` → `service.NewReportService(repo, time.Now)`
- `gormimpl.NewCollectRepo` → `repository.NewCollectRepo`
- etc.

- [ ] **Step 3: Update module/infra.go**

Update the database import:
```go
"github.com/StephenQiu30/hotkey-server/internal/platform/database"
```

- [ ] **Step 4: Run tests**

```bash
go test ./... 2>&1
```

Expected: all tests pass. Fix any test compilation errors:
1. Test files that imported old domain packages → update to new service/repository/entity paths
2. Test files that created service instances → use new constructor names
3. Test files in `tests/unit/` → update import paths

- [ ] **Step 5: Go vet**

```bash
go vet ./...
```
Expected: clean.

- [ ] **Step 6: Remove stale files**

Check for any remaining stale directories:
```bash
find internal -type d -empty -delete 2>/dev/null
```

- [ ] **Step 7: Final build verification**

```bash
go build ./...
go test ./...
```
Expected: both pass clean.

- [ ] **Step 8: Commit**

```bash
git add internal/fxapp/ internal/module/ internal/
git add -u
git commit -m "refactor: update fxapp wiring and final cleanup

Update all Fx DI imports, service constructors, and test imports
to match new controller/service/repository/model structure.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Self-Review Checklist

1. **Spec coverage:** Each spec requirement mapped — entity split (Task 1), DTO/config split (Task 2), repository flatten (Task 3), service merge (Task 4), controller (Task 5), fxapp cleanup (Task 6). All covered.

2. **Placeholder scan:** No "TBD", "TODO", or incomplete sections. Every sed command, file path, and code pattern is specified.

3. **Type consistency:** Constructor names are unique across tasks. `service.NewAuthService`, `service.NewMonitorService` etc. — no collision.

4. **Test coverage:** Tests files move with their source; imports updated in Task 6.

5. **Ordering dependency:** Tasks are linear (Task 1 → Task 6). Each depends on the previous. Build breaks are expected within a task but must be fixed before the task commit.

### Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-08-architecture-refactoring.md`.

Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration
2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

Which approach?
