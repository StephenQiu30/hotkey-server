# Architecture Refactoring: MVC + Layered Restructure

> Refactoring hotkey-server from domain-based packaging to standard MVC layered architecture.

**Date:** 2026-07-08
**Author:** StephenQiu30
**Status:** Draft

## Problem Statement

Current codebase uses domain-based packages (`internal/auth/`, `internal/monitor/`, `internal/report/`, etc.) where each domain mixes HTTP handlers, business logic, data models, and DTOs in the same package. This causes:

1. **Blurred layering** — HTTP handler code lives alongside business logic in `internal/platform/http/` (16 files), while GORM entities live in `internal/repository/gormimpl/model.go` (29 models in one file)
2. **Cross-cutting confusion** — `internal/database/` contains both DB connection bootstrap and read-query services; `internal/repository/gormimpl/` has GORM implementations but non-GORM query services sit in `internal/database/`
3. **No clear VO/DTO boundary** — Request/response types, domain value objects, and GORM entities are spread across `auth/model.go`, `monitor/model.go`, `hotevent/model.go`, and `repository/gormimpl/model.go`
4. **Implicit knowledge** — A new contributor must read every domain package before understanding the layering; there is no structural enforcement

## Design

### Architecture

Standard MVC (Model-View-Controller) layered architecture:

```
Controller  →  Service  →  Repository  →  Model (Entity)
   (HTTP)       (Biz)       (Data)         (DB Mapping)
```

Plus supporting layers:
- `platform/` — infrastructure (DB connection, logging, HTTP middleware, runtime context)
- `queue/` — Kafka producer/consumer infrastructure (kept independent)
- `worker/` — cron/scheduled tasks (kept independent)
- `config/` — centralized configuration
- `pkg/` — shared utility functions
- `fxapp/` — Fx DI container wiring

### Target Directory Structure

```
internal/
├── controller/              ← HTTP handlers + route registration
│   ├── route.go             ← NewRouter() + all route registration
│   ├── auth.go              ← handleRegister, handleLogin
│   ├── monitor.go           ← handleCreateMonitor, handleListMonitors etc.
│   ├── report.go            ← handleCreateReport, handleListReports etc.
│   ├── content.go           ← handleMonitorPosts
│   ├── topic.go             ← handleMonitorTopics
│   ├── notify.go            ← handleListNotifications etc.
│   ├── trending.go          ← handleTrending, handleHotEvents
│   ├── health.go            ← handleHealthz
│   ├── request.go           ← Request structs (input validation)
│   └── response.go          ← Response helpers (NewSuccess, NewError, NewPage)
│
├── service/                 ← All business logic (flattened by domain)
│   ├── auth.go              ← AuthService (register, login, JWT)
│   ├── monitor.go           ← MonitorService (CRUD + scoring)
│   ├── report.go            ← ReportService (create, list, daily)
│   ├── topic.go             ← TopicService (query + cluster integration)
│   ├── trend.go             ← TrendService (snapshot build)
│   ├── hotevent.go          ← HotEventService
│   ├── notify.go           ← NotifyService
│   ├── collect.go           ← CollectService (hit collection)
│   ├── llm.go              ← LLM provider + chain
│   ├── embedding.go        ← EmbeddingService (ONNX + cosine)
│   └── obsidian.go         ← ObsidianPublishService
│
├── repository/              ← Data access (GORM + read-only queries)
│   ├── user.go             ← UserRepo
│   ├── monitor.go          ← MonitorRepo
│   ├── report.go           ← ReportRepo
│   ├── report_export.go    ← ReportExportRepo
│   ├── notify.go           ← NotifyRepo
│   ├── hotevent.go         ← HotEventRepo
│   ├── collect.go          ← CollectRepo
│   ├── match.go            ← MatchRepo
│   ├── topic_read.go       ← TopicQueryService (read-only)
│   ├── topic_write.go      ← TopicWriteRepo
│   ├── snapshot.go         ← SnapshotRepo
│   ├── trend.go            ← TrendQueryService (read-only)
│   ├── content.go          ← ContentQueryService (read-only)
│   └── knowledge_run.go    ← KnowledgeRunRepo
│
├── model/
│   ├── entity/              ← GORM database entities (one file per domain)
│   │   ├── user.go          ← User
│   │   ├── monitor.go       ← KeywordMonitor, MonitorRun, MonitorPostHit
│   │   ├── post.go          ← PlatformPost, PlatformAuthor
│   │   ├── topic.go         ← Topic, TopicPost, TopicSnapshot
│   │   ├── monitor_snapshot.go ← MonitorSnapshot
│   │   ├── event.go         ← HotEvent, HotEventPlatform
│   │   ├── report.go        ← Report, ReportExport
│   │   ├── alert.go         ← Alert, UserNotification, EmailDelivery
│   │   ├── theme.go         ← Theme, ThemeMembership
│   │   ├── annotation.go    ← EventAnnotation, TopicAnnotation
│   │   ├── export.go        ← TopicDailyExport, ExportBundle
│   │   ├── knowledge.go     ← KnowledgeRun, KnowledgeWritebackLog, KnowledgeObjectRevision
│   │   └── dead_letter.go   ← DLQRecord
│   │
│   ├── dto/                 ← Data transfer objects (service params, non-DB structs)
│   │   ├── auth.go          ← RegisterRequest, LoginRequest, TokenResponse
│   │   ├── monitor.go       ← MonitorThresholdConfig, MonitorAlert
│   │   ├── hotevent.go      ← HeatScoreInput, EventQueryParams
│   │   ├── report.go        ← ReportFilterOptions
│   │   ├── notify.go        ← NotificationStatus
│   │   ├── embedding.go     ← VectorResult, MatchResult
│   │   ├── collect.go       ← HitResult, PostCandidate
│   │   └── obsidian.go      ← PublishResult, VaultEntry
│   │
│   └── vo/                  ← View objects (controller→frontend)
│       ├── auth.go          ← UserProfileVO, LoginVO
│       ├── monitor.go       ← MonitorListVO, MonitorDetailVO
│       ├── report.go        ← ReportListVO, ReportDetailVO
│       ├── trend.go         ← TrendingVO, TopicHeatVO
│       └── common.go        ← PageVO, ErrorVO
│
├── platform/                ← Infrastructure (unchanged responsibility)
│   ├── http/                ← Middleware, accesslog, swagger types ONLY
│   ├── database/            ← DB bootstrap, GORM connection (from internal/database/)
│   ├── logging/             ← Zap logger setup (unchanged)
│   └── runtime/             ← Context helpers (unchanged)
│
├── config/                  ← Config struct (split by domain)
│   ├── config.go            ← Master Config struct
│   ├── server.go            ← HTTP/Server config
│   ├── database.go          ← DB connection config
│   ├── kafka.go             ← Kafka brokers/group config
│   ├── redis.go             ← Redis addr config
│   ├── llm.go              ← LLM API config
│   └── obsidian.go          ← Obsidian vault path config
│
├── queue/                   ← Kafka infrastructure (unchanged)
├── worker/                  ← Cron/scheduled tasks (unchanged)
├── fxapp/                   ← Fx DI wiring (imports updated)
└── pkg/                     ← Shared utilities (unchanged)
```

### Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| `controller/` gets NewRouter() + routes | Controller owns the HTTP mapping; platform/http/ only provides middleware/tools |
| All business logic flattens into `service/` | Single import path for Fx wiring; cross-service calls use one package |
| Read-only query services → `repository/` | Consistent data access pattern; no distinction between read/write repos at package level |
| `model/dto/` for non-DB structs | Separates ORM concerns from domain/transfer concerns |
| `model/vo/` for API response shapes | Isolates frontend contracts from internal data structures |
| Config split into sub-files by domain | Each domain's config near its struct definition; avoids 30-field single file |
| `queue/` and `worker/` stay | These are infrastructure orchestrators, not business logic |

## Variables / Configuration

No new configuration values. The `config.Config` struct is split into sub-files by domain (`server.go`, `database.go`, `kafka.go`, `redis.go`, `llm.go`, `obsidian.go`) for organization — all fields remain in the same master struct.

## Migration Plan

### Step 1: Create directories

```bash
mkdir -p internal/{controller,service,repository,model/{entity,dto,vo},config}
```

### Step 2: Move `model/` — zero dependencies

Split `repository/gormimpl/model.go` (29 entities) into `model/entity/` files by domain.
Move DTO structs from domain packages into `model/dto/`.
Create `model/vo/` for response types.
Update all import references from `repository/gormimpl` → `model/entity`.

**Also in this step:** Split `config/config.go` (single 30+ field struct) into domain sub-files under `config/`. The master `Config` struct stays in `config.go` but each domain's fields move to their own file (`server.go`, `database.go`, `kafka.go`, etc.). Same package, no import changes needed.

### Step 3: Move `repository/` — depends on model/entity only

Flatten `repository/gormimpl/*` into `repository/` (same package).
Move `database/contentquery.go`, `database/topicquery.go`, `database/trendquery.go` into `repository/`.
**Also in this step:** Move DB bootstrap files (`database/bootstrap.go`, `database/database.go`, `database/logger.go`) into `platform/database/`.
Update all imports.

### Step 4: Move `service/` — depends on repository and model

Flatten `auth/`, `monitor/`, `report/`, `topic/`, `trend/`, `hotevent/`, `notify/`,
`llm/`, `embedding/`, `collect/`, `obsidian/` business logic into `service/` by domain file.

Each domain file exports the same Service struct(s) as before — no interface changes.

### Step 5: Move `controller/` — depends on service and model/vo

Move handler functions + `NewRouter()` from `platform/http/` to `controller/`.
Move request/response types to `model/vo/` and `controller/request.go`.
Keep middleware, accesslog, swagger_types in `platform/http/`.

### Step 6: Update `fxapp/app.go`

Update all import paths in the Fx DI wiring to point to new locations.

**Guarantee per step:** `go build ./...` must pass before moving to next step.
**Traceability:** Each step is one git commit.
**Zero logic changes:** Only import paths, package names, and file locations.

## Testing Strategy

- Unit tests move with their implementation files (same package name, same test logic)
- Test files in `tests/unit/` mirror the new structure
- `go test ./...` verifies correctness after Step 6
- No new tests needed — structural refactoring only

## Time Estimation

| Step | Scope | Est. |
|------|-------|------|
| Step 2 | model/entity (13 files) + model/dto (8 files) + model/vo (5 files) | ~20 min |
| Step 3 | repository/ (12 files from gormimpl + 3 from database) | ~15 min |
| Step 4 | service/ (12 files from 11 domains) | ~20 min |
| Step 5 | controller/ (10 handler files + route.go) | ~15 min |
| Step 6 | fxapp/app.go update + build fix | ~10 min |
| **Total** | | **~80 min** |
