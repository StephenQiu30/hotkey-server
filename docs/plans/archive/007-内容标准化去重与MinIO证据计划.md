---
layer: Plan
doc_no: "007"
audience: [Dev, QA, Ops]
feature_area: 内容与证据
purpose: 以 durable capture 实施 Content 标准化、确定性去重、MinIO 证据与删除同步
canonical_path: docs/plans/archive/007-内容标准化去重与MinIO证据计划.md
status: archived
execution_status: done
review_status: approved
version: v1.6
owner: HotKey Server Team
inputs:
  - docs/prd/archive/007-内容标准化去重与MinIO证据.md
  - docs/plans/archive/002-单一Schema与数据库平台计划.md
  - docs/plans/archive/006-查询规划与RSS-HN采集计划.md
  - docs/design/archive/003-数据库与数据生命周期设计.md
  - docs/design/archive/006-内容标准化去重与证据设计.md
outputs:
  - ingestion 模块和 Source capture 读取边界
  - Content/asset/metric 持久化与 MinIO 证据适配器
  - 安全 Content 查询与长期验收证据
triggers:
  - PRD-007 accepted 且 ready
downstream:
  - docs/acceptance/archive/007-内容标准化去重与MinIO证据验收.md
depends_on: [PLAN-002, PLAN-006]
---

# 内容标准化、去重与 MinIO 证据实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` task-by-task. Steps use checkbox syntax for durable progress.

**Goal:** 将 PLAN-006 已持久化的许可 CapturedItem 幂等转为可查询的 active Content，并保存可核验的 MinIO 文本证据、去重事实与删除状态。

**Architecture:** `source` 继续拥有 collection run/item 表；它通过窄 application reader 返回未绑定的捕获项，并在成功后仅绑定 `content_id` 或写入稳定 ingestion 失败码。`ingestion` 负责标准化、确定性去重、Content/asset/metric 的事务和生命周期，MinIO 只由其基础设施适配器调用。没有任何步骤重新 Fetch 来源、启动 River/Cron、创建 Embedding 或读写 Event/AI/Knowledge/Report 所有的表。

**Tech Stack:** Go 1.26、PostgreSQL/pgx Runtime、Gin、Swaggo、MinIO Go Client、标准库 `net/url`、`unicode/utf8`、`golang.org/x/text/unicode/norm`；真实 PostgreSQL 与可丢弃 MinIO integration fixture。

## 全局约束

- 代码开工前，Design-003 v3.2、Design-006 v1.4、PRD-007 与本 Plan 必须为 `accepted`，本 Plan 必须 `approved/ready`；全部 Task、Acceptance-007 与独立最终复核完成后，本 Plan 于 2026-07-17 归档为 `archived/done`。
- 只处理 `collection_run_items` 已持久化的捕获项；原始 Connector response 在 PLAN-006 已丢弃，禁止补抓或把它伪装成证据。
- `allow_body_storage=false` 时正文保持空且不写对象；`true` 时只写已持久化、清洗后的正文为 `content_assets.asset_type='text'`。
- 指标 `nil` 是未知，显式 `0` 是零；Content 当前值与每条 metric snapshot 的四个统一指标都必须以 SQL NULL 保存未知值。
- PLAN-007 近重复只用 `near_text-v1` 的严格确定性文本规则，且仅在同一 source connection 内比较；跨来源只允许 exact URL/hash。Embedding、跨语言语义和任务调度属于后续 Plan。
- 所有业务状态、Schema、records、OpenAPI、测试、验收与 README 同步；不新增 migration、AutoMigrate、动态规则引擎、通用工作流或来源专属表。

## 文件地图

| 动作 | 路径 | 职责 |
|---|---|---|
| 修改 | `docs/design/archive/003-数据库与数据生命周期设计.md`, `docs/design/archive/006-内容标准化去重与证据设计.md`, `docs/prd/archive/007-内容标准化去重与MinIO证据.md` | durable capture、未知指标、去重和证据边界 |
| 创建 | `docs/operations/plan007-schema-upgrade.md` | 既有库的备份、受控回填、legacy-zero、验证和回退运行手册；不构成第二份 Schema |
| 修改 | `db/schema.sql`, `internal/platform/database/model/{model,model_test}.go`, `internal/platform/database/database_integration_test.go`, `test/architecture/schema_test.go`, `test/tools/verify-schema.sh` | nullable metrics、dedupe metadata、ingestion status 和记录映射 |
| 修改 | `internal/modules/source/domain/{collection,ports}.go`, `internal/modules/source/application/captured_item_reader.go`, `internal/modules/source/infrastructure/postgres/{collection_record,collection_repository}.go` | Source-owned capture read/bind/failure port |
| 创建 | `internal/modules/ingestion/domain/{content,ports,errors}.go` | Content、去重结果、EvidenceStore、Repository 与生命周期契约 |
| 创建 | `internal/modules/ingestion/application/{normalizer,deduplicator,service,lifecycle}.go` | 规范化、确定性去重、批处理和删除对账用例 |
| 创建 | `internal/modules/ingestion/infrastructure/postgres/{record,repository}.go` | authors/contents/assets/snapshots 的 PostgreSQL 实现 |
| 创建 | `internal/modules/ingestion/infrastructure/minio/store.go`, `历史 MinIO fixture 脚本（已移除）` | MinIO EvidenceStore 与仓库受控的 disposable MinIO 启停夹具，不修改 `.env`/`.env.prod` |
| 修改 | `internal/platform/config/{config,config_test}.go`, `.env.example`, `go.mod`, `go.sum`, `internal/bootstrap/app.go` | MinIO 配置校验、依赖装配和模块注册 |
| 创建 | `internal/modules/ingestion/transport/http/{dto,handler,routes}.go` | viewer-safe Content list/detail，无 asset 下载路由 |
| 修改 | `docs/openapi/swagger.json`, `test/architecture/{openapi_test,platform_identity_boundary_test}.go` | Content API 和跨模块 SQL 边界 |
| 创建 | `docs/acceptance/archive/007-内容标准化去重与MinIO证据验收.md` | RED/GREEN、对象存储、独立复核与归档证据 |

## Task 1：Schema、capture 读取边界与未知指标契约

**Consumes:** Design-003 v3.2、Design-006 v1.4、PLAN-006 durable capture。

**Produces:** nullable Content/metric-snapshot 指标、`dedupe_reason`/`dedupe_version`、capture ingestion 状态及来源归属约束，以及 Source application 的窄 `CapturedItemReader`。

**Files:** Modify `db/schema.sql`, `internal/platform/database/model/{model,model_test}.go`, `internal/platform/database/database_integration_test.go`, `test/architecture/schema_test.go`, `test/tools/verify-schema.sh`, `internal/modules/source/domain/{collection,ports}.go`, `internal/modules/source/application/captured_item_reader.go`, `internal/modules/source/infrastructure/postgres/{collection_record,collection_repository,collection_repository_integration_test}.go`, `internal/modules/source/{domain,application}/*_test.go`, `docs/operations/README.md`; Create `docs/operations/plan007-schema-upgrade.md`.

**Interfaces:**

```go
type CapturedItemReader interface {
    ListUnboundCaptured(context.Context, CapturedItemQuery) (CapturedItemPage, error)
    BindContent(context.Context, CapturedContentBinding) error
    MarkIngestionFailure(context.Context, CapturedIngestionFailure) error
}

type CapturedItemQuery struct { RunID int64; Cursor string; Limit int; IncludeFailed bool }
type CapturedItemPage struct { Items []CapturedCollectionItem; NextCursor string }
type CapturedCollectionItem struct {
    ID, RunID, SourceConnectionID int64
    Item source.CapturedItem
}
type CapturedContentBinding struct { CollectionItemID, RunID, SourceConnectionID, ContentID int64 }
type CapturedIngestionFailure struct { CollectionItemID, RunID, SourceConnectionID int64; Code string }

type SourceMetrics struct {
    ViewCount, LikeCount, CommentCount, ShareCount *int64
}
```

- [x] **RED:** 写真实 PostgreSQL 负例，断言 `contents` 与 `content_metric_snapshots` 的 `NULL` 指标都不被默认成 0、显式 0 仍保存 0、duplicate 缺 reason/version 被拒绝、跨 run/跨 source/已绑定 item 不能二次绑定、失败 ingestion 不改变 collection target capture outcome；另构造 PLAN-006 v1 `collection_run_items`/旧 0 metrics，验证受控升级可回填 source、将不具存在性信息的零保守改为 unknown、保持正值且可回退。
- [x] **运行 RED:** `HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' go test ./internal/platform/database ./internal/modules/source/... ./test/architecture -run 'Test(Content|Captured|Collection|Plan007SchemaUpgrade).*' -count=1`；重点运行 `HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' go test ./internal/platform/database -run TestPlan007SchemaUpgradeBackfillsCaptureSourceAndPreservesMetricPolicy -count=1`；旧 Schema/port 必须因列、类型或方法缺失失败。
- [x] **GREEN:** `contents` 与 `content_metric_snapshots` 的四个统一指标改为 nullable 并有 nonnegative CHECK；records 用 `*int64`/`sql.NullInt64` 保持 null。duplicate 必有 target/reason/version；`contents` 新增 `UNIQUE(id, source_connection_id)`，`collection_runs` 新增 `UNIQUE(id, source_connection_id)`，`collection_run_items` 固化 `source_connection_id`，以 `(run_id, source_connection_id)` 和 `(content_id, source_connection_id)` 复合 FK 同时验证 run 与 Content 归属。`collection_run_items` 增加独立 `ingestion_status pending/succeeded/failed` 与稳定 `ingestion_error_code`。Source repository 以 join `collection_runs` 返回仅 `outcome='captured'`、`content_id is null`、状态为 pending（明确重试时可含 failed）的 safe persisted item；游标是 `id ASC` keyset。`BindContent` 在 callback context 的现有事务中以 `id/run/source/content_id is null/status in (pending,failed)` compare-and-set 绑定、改为 succeeded 并清空错误；`MarkIngestionFailure` 只改 ingestion 状态/错误，绝不改 capture outcome/target。新 capture 改为 CapturedItem v2，以 `*int64` 编码 nil/0；reader 仍接受 v1，但把 v1 的零指标保守映射为 nil、正值保持。
- [x] **受控升级与回退:** 创建运行手册而非 runtime/migration 代码：维护窗口前执行 `pg_dump "$HOTKEY_DATABASE_URL" --format=custom --file=/secure-backups/hotkey-before-plan007.dump` 并以 `pg_restore --list` 验证；在已恢复的副本先演练从 `collection_runs` 回填 `collection_run_items.source_connection_id`，验证无 null/错属，再加非空/复合键。所有既有 `contents` 与 `content_metric_snapshots` 的零用 `NULLIF(value, 0)` 保守转换为 unknown，正值不变；v1 CapturedItem 不重抓。演练后运行 `HOTKEY_DATABASE_URL="$HOTKEY_DATABASE_URL" go run ./cmd/hotkey db verify`、该 upgrade integration 与行数/正值聚合对账。任何一步失败，停止写入并用经验证的 custom backup 恢复；手册 SQL 只从本任务的 `db/schema.sql` 派生，不能成为第二份 Schema。
- [x] **回归:** `sh test/tools/verify-schema.sh`、`HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' go test -race ./internal/platform/database/... ./internal/modules/source/... ./test/architecture -count=1`。
- [x] **提交:** `feat: add ingestion capture boundary`。

## Task 2：Content 领域、规范化与确定性三层去重

**Consumes:** Task 1 `CapturedItemReader` 输出和 nullable metrics。

**Produces:** 无 HTTP/SQL/MinIO 依赖的 `NormalizedContent`、`DedupeDecision` 与稳定错误码。

**Files:** Create `internal/modules/ingestion/domain/{content,ports,errors}.go`, `internal/modules/ingestion/application/{normalizer,deduplicator}.go` and their `*_test.go`; Modify `internal/modules/source/domain/{collection,connector}.go` and connector tests for optional metrics.

**Interfaces:**

```go
type DedupeDecision struct {
    Status ContentStatus
    DuplicateOfID *int64
    Reason, Version string
}

type NormalizedAuthor struct { ExternalID, DisplayName string }
type NormalizedContent struct {
    SourceConnectionID int64
    ExternalID, ContentType, Title, Excerpt, Body, CanonicalURL, Language string
    Author NormalizedAuthor
    PublishedAt, FetchedAt time.Time
    ContentHash string
    Metrics source.SourceMetrics
}
type ContentCandidate struct {
    ID, SourceConnectionID int64
    PublishedAt time.Time
    TitleTokens, BodyTokens []string
    CanonicalURL, DedupeKey string
}

func NormalizeCapturedItem(source.CapturedItem, int64) (NormalizedContent, error)
func DecideDuplicate(NormalizedContent, []ContentCandidate) (DedupeDecision, error)
```

- [x] **RED:** 表驱动测试覆盖 NFC、HTML/script/control-character 清理、URL host/scheme/tracker 规范化、empty title+body、invalid URL、`comment -> post`、nil vs explicit-zero metric、same source retry、canonical URL、content hash、正文/作者/发布时间/观察时间/指标无丢失地进入 `NormalizedContent`、near_text 0.98、跨来源 100/102 token 临界独立报道与跨语言报道不折叠。
- [x] **运行 RED:** `go test ./internal/modules/ingestion/domain ./internal/modules/ingestion/application -run 'Test(Normalize|Decide)' -count=1`；实现前应因 package/type 缺失失败。
- [x] **GREEN:** `NormalizeCapturedItem` 将已持久化 body（仅非空时可作证据）、作者投影、`PublishedAt`（缺失时取 `ObservedAt`）、`FetchedAt=ObservedAt`、内容哈希与 `SourceMetrics` 不可变地带入结果；作者 external ID 由同一 source 下规范化作者标识的 SHA-256 稳定推导。`near_text-v1` 仅在同一 `source_connection_id`、发布时间相差不超过 24 小时、规范化标题 token 完全一致、两个非空正文 token Jaccard 至少 0.98 时返回 duplicate；否则保持 active。URL 和 exact hash 可跨来源优先匹配，所有 duplicate 返回目标 ID、`exact_url`/`exact_hash`/`near_text` 和版本；不创建 Embedding、实体或来源 HTTP 调用。
- [x] **回归:** `go test -race ./internal/modules/ingestion/domain ./internal/modules/ingestion/application -count=1`。
- [x] **提交:** `feat: define normalized content deduplication`。

## Task 3：PostgreSQL Content、作者、资产与快照 Repository

**Consumes:** Tasks 1–2 的类型和决策。

**Produces:** Content 的 source-idempotent upsert、cursor read、asset state 与 nullable metric snapshot 事务端口。

**Files:** Create `internal/modules/ingestion/infrastructure/postgres/{record,repository,repository_integration_test}.go`; Modify `internal/modules/ingestion/domain/ports.go`.

**Interfaces:**

```go
type ContentRepository interface {
    Upsert(context.Context, NormalizedContent, DedupeDecision) (Content, bool, error)
    AppendMetricSnapshot(context.Context, contentID int64, capturedAt time.Time, metrics source.SourceMetrics) error
    CreateAsset(context.Context, ContentAsset) error
    MarkAssetStatus(context.Context, objectKey string, status AssetStatus) error
    ListActive(context.Context, ContentListQuery) (ContentPage, error)
    MarkDeleted(context.Context, sourceConnectionID int64, externalID string) (Content, bool, error)
}
```

- [x] **RED:** PostgreSQL integration tests覆盖 source external-id 并发 upsert、作者稳定 ID、duplicate metadata、Content 与每条 snapshot 的 nullable/zero metrics、cursor 不重复、asset object-key 唯一、版本冲突、deleted 内容不出现在 active list。
- [x] **运行 RED:** `HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' go test ./internal/modules/ingestion/infrastructure/postgres -run TestContentRepository -count=1`；旧仓库不存在时失败。
- [x] **GREEN:** 所有 SQL 参数化；source retry 只更新同一 Content 的 `fetched_at`、指标、对应 snapshot 与 version；`AppendMetricSnapshot` 保持 `nil -> SQL NULL`、显式 `0 -> SQL 0`；active list 固定 `(published_at DESC,id DESC)` 游标并永远过滤 `content_status='active' AND deleted_at IS NULL`；asset 记录绝不存 access key、secret 或正文。
- [x] **回归:** `HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' go test -race ./internal/modules/ingestion/infrastructure/postgres -count=1`。
- [x] **提交:** `feat: persist normalized content`。

## Task 4：MinIO EvidenceStore 与对象一致性

**Consumes:** Task 3 asset state、`MinIOConfig`。

**Produces:** 一个 ingestion-owned `EvidenceStore` 实现、确定性正文对象键和 orphan reconciliation。

**Files:** Create `internal/modules/ingestion/infrastructure/minio/{store,store_test,store_integration_test}.go`, `历史 MinIO fixture 脚本（已移除）`; Modify `internal/modules/ingestion/domain/ports.go`, `internal/platform/config/{config,config_test}.go`, `.env.example`, `go.mod`, `go.sum`.

**Interfaces:**

```go
type EvidenceStore interface {
    PutText(context.Context, EvidenceObject) (EvidenceReceipt, error)
    Delete(context.Context, string) error
    ListPrefix(context.Context, string) ([]EvidenceReceipt, error)
}

func EvidenceObjectKey(sourceID int64, sha256 string) string
```

- [x] **RED:** fake-store unit tests覆盖相同 SHA 幂等上传、SHA 不匹配、timeout、Delete failure、空正文禁止上传；real MinIO integration 覆盖 bucket/object `Head` 校验、deterministic-key reuse 及对象 metadata/size/SHA 一致性。应用级补偿不在本 Task 以 fake 冒充，必须在 Task 5 使用真实 PostgreSQL 与 MinIO 验证。
- [x] **运行 RED:** 先执行 `外部 disposable MinIO fixture 已启动`；随后运行 `HOTKEY_TEST_MINIO_ENDPOINT=127.0.0.1:19007 HOTKEY_TEST_MINIO_ACCESS_KEY=hotkey-plan007 HOTKEY_TEST_MINIO_SECRET_KEY=hotkey-plan007-secret HOTKEY_TEST_MINIO_BUCKET=hotkey-plan007 go test -tags=integration ./internal/modules/ingestion/infrastructure/minio -count=1`，并始终执行 `外部 disposable MinIO fixture 已停止`；unit command 为 `go test ./internal/modules/ingestion/infrastructure/minio -run TestStore -count=1`。
- [x] **GREEN:** `外部 disposable MinIO fixture` 只启动/停止名为 `hotkey-plan007-minio-fixture` 的 disposable Docker MinIO，固定测试端口 `19007`，不读取或改写 `.env`/`.env.prod`，且 `up/down` 可重复执行。integration test 自行以 `MakeBucket` 的 already-exists 处理创建 bucket，并为每例创建唯一 SourceConnectionID；对象仅写入该 source 的 `evidence/v1/{sourceID}/` 前缀，在 `t.Cleanup` 清理该前缀。key 为 `evidence/v1/{sourceID}/{sha256[:2]}/{sha256}.txt`；Put 后必须 Head 验证 SHA-256/size；运行配置要求 endpoint、bucket、access key 和 secret 都非空，但错误与日志不回显 secret；不创建第二个对象存储抽象或全局 client。
- [x] **回归:** fake、在上述 repo fixture 上运行的真实 MinIO integration 与 `go test ./internal/platform/config -count=1` 都通过。
- [x] **提交:** `feat: add minio evidence store`。

## Task 5：Ingestion 编排、证据补偿与 per-item 隔离

**Consumes:** Tasks 1–4。

**Produces:** `IngestRun`，将每个 capture 独立标准化、持久化、上传/补偿并 bind 回 Source。

**Files:** Create `internal/modules/ingestion/application/{service,service_test,service_integration_test}.go`; Modify `internal/bootstrap/app.go` for constructor wiring only.

**Interfaces:**

```go
type IngestRunInput struct { RunID int64; Limit int }
func (s *Service) IngestRun(context.Context, IngestRunInput) (IngestRunResult, error)
```

- [x] **RED:** 真实 PostgreSQL + fake EvidenceStore unit tests覆盖两 item 其中一个解析失败、同一 run 重放、许可 body 的 Put→Head→asset→bind、无许可 body 无 Put、不得调用 Connector。另写不可跳过的 real PostgreSQL + real MinIO application integration：用注入的 Source bind failure 强制 asset 写入后整个 `Runtime.WithinTransaction` 回滚，断言无 asset/bind 且对象被 Delete；用只在第一次 Delete 返回错误的 real-MinIO decorator 制造无引用对象，随后用实际 `ReconcileObjects` 删除它。两例都必须检查对象 `Head`/ListPrefix 和数据库状态。
- [x] **运行 RED:** 单元：`HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' go test ./internal/modules/ingestion/application -run TestIngestRun -count=1`；真实组合在 Task 4 fixture 已 `up` 后运行：`HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' HOTKEY_TEST_MINIO_ENDPOINT=127.0.0.1:19007 HOTKEY_TEST_MINIO_ACCESS_KEY=hotkey-plan007 HOTKEY_TEST_MINIO_SECRET_KEY=hotkey-plan007-secret HOTKEY_TEST_MINIO_BUCKET=hotkey-plan007 go test -tags=integration ./internal/modules/ingestion/application -run 'TestIngestRun(MinIOPostgresRollbackDeletesObject|MinIOPostgresReconcileDeletesOrphan)' -count=1`；初始因 Service/Source reader 缺失失败。
- [x] **GREEN:** 每条 item 使用单独 `database.Runtime.WithinTransaction`；所有 Content repository 与 Source bind 都接收同一个 callback context 并通过 `database.TransactionFromContext` 复用该 SQL transaction。失败调用 `MarkIngestionFailure` 的受控 code 后继续处理下一条；对象上传在 DB 事务外，asset 引用和 bind 在同一 DB transaction 内；DB 失败立即 Delete，删除失败只形成无引用 orphan，reconciler 以 prefix/已知 asset keys 清除。不得启动 River/Cron 或重新调用 Source Connector。
- [x] **回归:** `HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' go test -race ./internal/modules/ingestion/... ./internal/modules/source/... -count=1`，并运行上述真实 PostgreSQL + MinIO command。
- [x] **提交:** `feat: ingest captured content evidence`。

## Task 6：删除、过期与对象对账

**Consumes:** Task 5 Content/asset states。

**Produces:** 幂等 `DeleteBySourceItem`、`ExpireBefore` 与 `ReconcileObjects` application commands；仅影响 ingestion-owned Content/asset 事实。

**Files:** Create `internal/modules/ingestion/application/{lifecycle,lifecycle_test,lifecycle_integration_test}.go`; Modify `internal/modules/ingestion/{domain/ports.go,infrastructure/postgres/repository.go}`.

- [x] **RED:** 测试重复删除、asset Delete 成功/失败、expired active Content、deleted/expired cursor exclusion、reconcile 删除无 DB 引用对象但不删除 available asset，以及不存在 source item 的幂等 no-op。
- [x] **运行 RED:** `HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' go test ./internal/modules/ingestion/application -run 'Test(Delete|Expire|Reconcile)' -count=1`。
- [x] **GREEN:** 先将 Content 标为 deleted/expired 并使读模型立即排除；asset 删除失败时写 `delete_pending`，重试成功写 `deleted`；`ReconcileObjects` 只检查 `evidence/v1/` 前缀和 ingestion asset keys。不得删除 Event/Match/AI/Knowledge/Report 表或假装这些下游已重算。
- [x] **回归:** `HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' go test -race ./internal/modules/ingestion/... -count=1`。
- [x] **提交:** `feat: synchronize content evidence lifecycle`。

## Task 7：安全 Content 查询 HTTP、OpenAPI 与可观测性

**Consumes:** Task 3 active cursor read。

**Produces:** `GET /api/v1/contents`、`GET /api/v1/contents/{id}`，只返回 active safe Content；无对象下载路由。

**Files:** Create `internal/modules/ingestion/transport/http/{dto,handler,routes,handler_test,handler_integration_test}.go`; Modify `internal/bootstrap/app.go`, `docs/openapi/swagger.json`, `test/architecture/openapi_test.go`, `internal/platform/observability/{metrics,metrics_test}.go`.

- [x] **RED:** handler tests覆盖未认证 401、viewer/admin 200/`code:0`、cursor 输入 400、not-found/deleted 404、response 不含 asset object key、MinIO endpoint、credential、正文或错误堆栈。
- [x] **运行 RED:** `go test ./internal/modules/ingestion/transport/http -run TestContentRoutes -count=1`；未定义 routes/DTO 时失败。
- [x] **GREEN:** transport 只调用 ingestion application；DTO 只含 id、source type/name、external ID、content type、title、canonical URL、language、published/fetched times、nullable metrics 和 dedupe status/reason/version；metrics 只用 operation/outcome，OpenAPI 生成后无漂移。
- [x] **回归:** `HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' go test ./internal/modules/ingestion/transport/http ./test/architecture -count=1 && make openapi-validate`。
- [x] **提交:** `feat: expose safe content queries`。

## Task 8：受控验收、独立复核与归档

**Goal:** 形成 Acceptance-007，并仅在 PostgreSQL/MinIO/HTTP 证据与独立复核均通过后归档。

**Files:** Create `docs/acceptance/archive/007-内容标准化去重与MinIO证据验收.md`; Modify `docs/acceptance/README.md`, `docs/operations/README.md`, `docs/prd/archive/007-内容标准化去重与MinIO证据.md`, `docs/plans/archive/007-内容标准化去重与MinIO证据计划.md`, indexes, `docs/README.md`, `README.md`.

- [x] **RED:** 保存 Tasks 1–7 实际缺失 schema/metric/capture/object/permission 信号；禁止事后伪造。
- [x] **GREEN:** 先执行 `外部 disposable MinIO fixture 已启动`，用可丢弃 PostgreSQL、Redis 与该 repo fixture 运行所有 Task 回归、`make ci`、schema/OpenAPI 验证和 Gin runtime HTTP；明确运行 `HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' HOTKEY_TEST_MINIO_ENDPOINT=127.0.0.1:19007 HOTKEY_TEST_MINIO_ACCESS_KEY=hotkey-plan007 HOTKEY_TEST_MINIO_SECRET_KEY=hotkey-plan007-secret HOTKEY_TEST_MINIO_BUCKET=hotkey-plan007 go test -tags=integration ./internal/modules/ingestion/application -run 'TestIngestRun(MinIOPostgresRollbackDeletesObject|MinIOPostgresReconcileDeletesOrphan)' -count=1`，并保存真实 `Head`/ListPrefix、无 asset/bind 与对账删除证据；结束时无论成功或失败都运行 `外部 disposable MinIO fixture 已停止`。在可恢复副本上按 `docs/operations/plan007-schema-upgrade.md` 完整演练，并保存 backup-list、回填/legacy-zero/正值聚合与 `db verify` 证据。确认 no-refetch、Content/snapshot unknown-vs-zero、跨来源临界 independent-report、object compensation、同来源/状态机绑定、deletion/idempotency、safe DTO。
- [x] **独立复核:** 非主要编写者检查设计/PRD/Plan、所有 007 提交、Source→Ingestion 边界、Schema/records、MinIO 故障、事务竞态、HTTP/OpenAPI、日志/指标脱敏、Acceptance 与 clean worktree。Critical/Important 必须修复并重跑。
- [x] **归档:** 复核通过后 Acceptance-007 为 accepted，PRD/Plan-007 改 `archived/done`；PLAN-008 才成为自身审核后的 ready 候选。
- [x] **提交:** `git add docs README.md && git commit -m "docs: archive ingestion plan"`。

## 计划自检

- PRD 的来源幂等、去重解释、未知指标、许可正文、对象补偿、删除和安全读模型分别由 Tasks 1–7 覆盖。
- `SourceMetrics`、capture reader、dedupe decision、包含正文/作者/时间/哈希/指标的 NormalizedContent、Content repository、EvidenceStore 与 IngestRun 的名称、输入和输出在生产者任务先定义，后续任务只消费这些契约。
- 本 Plan 不含待填项、动态扩展点、Embedding、River、真实来源 Fetch 或跨模块 Event SQL；每一项风险均有可执行 RED/GREEN 或明确的 integration fixture。

## 独立审核记录

| 日期 | 结论 | 覆盖范围 |
|---|---|---|
| 2026-07-16 | approved | 独立 Reviewer 已完成三轮复核，确认 durable capture/no-refetch、nullable metrics、同来源 near-text、Source 归属和共享事务、既有库升级/回退、真实 MinIO fixture、删除所有权与状态门禁均无 Critical/Important。 |
| 2026-07-17 | approved | 非主要编写者最终复核 `b87111b..a7d112b`、Acceptance-007、真实 fixture/回退/Gin/CI/race、模块边界与 clean worktree；无未解决 Critical 或 Important。 |

## 风险与回滚

- 执行期间真实 MinIO fixture 不可用时，PLAN 必须保持 in_progress/blocked，不得用 fake 结果伪称对象存储验收；归档后发现对象存储回归时创建新的前向修复 Plan。
- 发现来源协议不允许的正文时，删除对应对象并保持仅元数据 Content，不修改 Connector 以重抓数据。
- 下游 Plan 开工前可用完整提交回退 Schema、records、ingestion 与 OpenAPI；下游开工后只用新的前向修复 Plan。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-16 | 初始高层执行提纲。 |
| v1.1 | 2026-07-16 | 收敛为 durable-capture P0 计划：补齐未知指标、去重、MinIO、Source 边界、删除、HTTP、红绿、独立复核与归档任务；等待独立审核。 |
| v1.2 | 2026-07-16 | 按独立复核收紧：补齐标准化结果与快照 nullable 契约、同来源 near-text、复合来源归属/状态机绑定，以及真实 PostgreSQL + MinIO 补偿与对账验收。 |
| v1.3 | 2026-07-16 | 补齐既有库升级/回退与 PLAN-006 legacy-zero 策略，并提供不引入新环境文件的仓库受控 MinIO fixture、唯一前缀隔离和 teardown。 |
| v1.4 | 2026-07-16 | 三轮独立复核通过，状态转为 accepted/approved/ready，可按 Task 1 开工。 |
| v1.5 | 2026-07-16 | PLAN-007 已激活为 in_progress；逐 Task 的执行流水保留在 Workpad、提交和最终 Acceptance，不改变已批准的实施契约。 |
| v1.6 | 2026-07-17 | Task 1–8、Acceptance-007 与独立最终复核完成，归档为 archived/done；PLAN-008 保持其自身 review/backlog/pending 门禁。 |
