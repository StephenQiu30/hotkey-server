---
layer: Plan
doc_no: "006"
audience: [Dev, QA, Ops]
feature_area: 来源采集
purpose: 以测试先行实施查询规划及 RSS、Atom、Hacker News 的共享捕获运行
canonical_path: docs/plans/006-查询规划与RSS-HN采集计划.md
status: accepted
execution_status: ready
review_status: approved
version: v1.8
owner: HotKey Server Team
inputs:
  - docs/prd/006-查询规划与RSS-HN采集.md
  - docs/plans/005-监控主题规则与来源配置计划.md
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/005-数据来源查询规划与采集设计.md
  - docs/design/012-监控调度与River流水线设计.md
  - docs/design/014-监控配置发布与预览设计.md
outputs:
  - Connector 契约
  - RSS、Atom、Hacker News 适配器
  - 采集运行与检查点
triggers:
  - PRD-006 accepted 且 ready
downstream:
  - docs/acceptance/006-查询规划与RSS-HN采集验收.md
depends_on: [PLAN-005]
---

# 查询规划与 RSS/HN 采集计划

**Goal:** 交付 RSS/Atom 与 Hacker News 的合规、增量、限流、可恢复共享捕获能力。完成时，已发布 Monitor 配置产生稳定查询，单次外部请求由一个 shared run 服务多个 immutable target，并把每页 SourceItem 持久化为 `collection_run_items`；Content 标准化、去重和 MinIO 证据仍属于 PLAN-007。

**Architecture:** `monitor` 通过 source-owned窄端口输出已发布的 collection target；`source` 负责 query planner、Connector、run/checkpoint domain、application service、PostgreSQL repository 与 admin-only transport。RSS/Atom 和 HN Connector 只返回 `SourceItem`，不创建 Content 或调用 River。`collection_runs` 的唯一键是 `(source_connection_id, query_signature, window_start, window_end)`；每个 target 单独记录结果，成功捕获只推进它自己的 fetch checkpoint。PLAN-013 才引入 Cron/River，因此本计划仅实现同步受控执行和安全重试入口。

**Tech Stack:** Go 1.26、现有 PostgreSQL/pgx/GORM Runtime、Gin、Zap、标准库 `net/http` 与 `encoding/xml`/`encoding/json`；RSS/Atom/HN 测试只使用本地 `httptest` fixture，不连接真实来源或秘密。

## 开工条件

- 当前 Plan 为 `status: accepted`、`review_status: approved`、`execution_status: ready`，对应 PRD 为 `accepted/ready`。
- PLAN-005 为 `archived/done`；Design-003、Design-005、Design-012 和 Design-014 均为 accepted。
- Git 基线同步，工作树只包含本计划文件；`HOTKEY_TEST_DSN` 与 `HOTKEY_TEST_REDIS_URL` 指向可丢弃 PostgreSQL+pgvector 与 Redis。
- 新增/修改公共 HTTP 契约前，先更新 Design-014、错误码、OpenAPI 与对应 transport 测试。

## 稳定接口与边界

| 接口 | 输入 | 输出 | 所属 |
|---|---|---|---|
| `PublishedCollectionTargetReader.ListDue` | 时间窗口 | immutable target、已签名 query、词项、语言/地区、checkpoint | Monitor adapter，实现 Source domain port |
| `QueryPlanner.Plan` | published target | stable `CollectionRequest`；不重算 Monitor 侧 `query_signature` | Source application |
| `Connector.Fetch` | `FetchRequest` | `FetchResult{Items, NextCursor, ETag, LastModified, RetryAfter}` | Source infrastructure |
| `CollectionRepository.CreateOrReuseRun` | request 四元组与 targets | shared run、target 行与是否执行 | Source PostgreSQL |
| `CollectionService.Collect` | `CollectionRequest` | captured run summary；不写 Content/MinIO/River | Source application |
| `CollectionRunService.Retry/Health` | admin subject、run/source ID | 安全 run/health DTO；不返回 endpoint、credential ref 或原始响应 | Source transport |

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 修改 | `docs/design/003-数据库与数据生命周期设计.md`, `docs/design/005-数据来源查询规划与采集设计.md`, `docs/design/012-监控调度与River流水线设计.md`, `docs/design/014-监控配置发布与预览设计.md` | 固化 Schema、capture/checkpoint、运行 API 和错误码边界 |
| 修改 | `db/schema.sql`, `internal/platform/database/model/model.go`, `internal/platform/database/model/model_test.go`, `internal/platform/database/database_integration_test.go`, `tests/architecture/schema_test.go`, `scripts/verify-schema.sh` | shared run、target、item、checkpoint 的完整 Schema/记录/门禁 |
| 创建 | `internal/modules/source/domain/{collection,connector,collection_errors}.go` 及 `*_test.go` | SourceItem、Connector、run/checkpoint 与 port 契约 |
| 创建 | `internal/modules/source/application/{query_planner,collection_service}_test.go` 及 `.go` | 查询规划、run 编排、重试分类与 target 隔离 |
| 创建 | `internal/modules/monitor/infrastructure/postgres/collection_target_reader.go` 及测试 | published Monitor 到 Source 的窄只读适配器 |
| 创建 | `internal/modules/source/infrastructure/{rss,hackernews}/*.go` 及 `testdata/sources/{rss,atom,hackernews}/*` | RSS/Atom 与 HN HTTP fixture Connector |
| 创建 | `internal/modules/source/infrastructure/postgres/{collection_record,collection_repository}.go` 及测试 | run、target、item、checkpoint 持久化 |
| 修改 | `internal/shared/errors/error.go`, `internal/shared/errors/error_test.go`, `internal/bootstrap/app.go`, `internal/platform/http/router.go` | 错误码、依赖装配和路由注册 |
| 创建 | `internal/modules/source/transport/http/{collection_dto,collection_handler,collection_routes}.go` 及测试 | admin run 列表、重试、health HTTP/OpenAPI |
| 修改 | `docs/openapi/swagger.json`, `tests/architecture/openapi_test.go`, `tests/architecture/platform_identity_boundary_test.go` | API、依赖方向和公开契约门禁 |
| 创建 | `docs/acceptance/006-查询规划与RSS-HN采集验收.md` | 完成后的长期红绿、环境、HTTP、复核和归档证据 |

## Task 1：共享捕获 Schema、记录模型与约束

**目标：** 让 shared run、target、captured item 和 fetch checkpoint 的状态可在空库中安全持久化与恢复。

**Consumes：** Design-003 的 Schema/生命周期契约、Design-005/012 的 shared request、target capture 与 checkpoint 规则；现有 PLAN-005 已建的基础表。

**Produces：** `collection_runs` 保存 request/response cursor、ETag、Last-Modified、page count、retry-after、错误分类和更新时间；target 保存捕获状态/计数/更新时间；item 保存 source code、content type、captured item version、可重放且脱敏的 `captured_item` JSON、payload hash、raw payload disposition、捕获结果和去重唯一键；`collection_run_target_items` 保存每个 target 对每个 item 的 capture reconciliation；checkpoint 保存最后成功 run 与最后 fetch 时间。

**Files：** Modify `docs/design/003-数据库与数据生命周期设计.md`, `db/schema.sql`, `internal/platform/database/model/model.go`, `internal/platform/database/model/model_test.go`, `internal/platform/database/database_integration_test.go`, `tests/architecture/schema_test.go`, `scripts/verify-schema.sh`.

- [ ] **RED：** 添加记录映射与真实 PostgreSQL 测试，要求上述 fields、`source_connection_id + query_signature + window` 唯一键、`run_id + external_id` 唯一键、captured item version/JSON/raw disposition、`collection_run_target_items` 的 target/item 唯一键、target immutable config 外键和 checkpoint 乐观版本；现有 Schema 必须因字段/约束缺失失败。
- [ ] **运行 RED：** `HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:5432/hotkey_test?sslmode=disable' go test ./internal/platform/database/model ./internal/platform/database ./tests/architecture -run 'TestCollection|TestCompleteSchemaCoversMappedRecords|TestGreenfieldSchemaEnforcesCriticalConstraints' -count=1`。
- [ ] **GREEN：** 只修改完整 `db/schema.sql` 与对应 records/`All()`/表数断言；capture payload 只保存已规范化的 SourceItem envelope，原始第三方响应字节不进 PostgreSQL；空库二次执行后验证同一 run 复用、跨 target 隔离、item 幂等、target-item 对账和失败 target 不推进 checkpoint。
- [ ] **重构：** 把 collection record mapping 与业务状态转换保持在 Source 模块，模型只保存列映射；不创建 migration、AutoMigrate 或临时 Schema。
- [ ] **回归：** `HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:5432/hotkey_test?sslmode=disable' sh scripts/verify-schema.sh`、`HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:5432/hotkey_test?sslmode=disable' sh scripts/verify-database-runtime.sh`。
- [ ] **提交：** `git add docs/design/003-数据库与数据生命周期设计.md db/schema.sql internal/platform/database/model internal/platform/database/database_integration_test.go tests/architecture/schema_test.go scripts/verify-schema.sh && git commit -m "feat: add collection capture schema"`。

## Task 2：采集领域契约与 published target 读取边界

**目标：** 定义不泄露 HTTP/SQL/secret 的 Connector、SourceItem、run/checkpoint、分类错误和 Monitor→Source 窄端口。

**Consumes：** Task 1 的 Schema 名称、Design-014 的 SourceConnection 安全模型、已发布 Monitor config。

**Produces：** `FetchRequest`、`FetchResult`、`SourceItem`、`CapturedItem`、`CapturePolicy`、`CollectionRequest`、`CollectionRun`、`CollectionTarget`、`CollectionTargetItem`、`CollectionCheckpoint`、`Connector`、`CollectionRepository` 与 `PublishedCollectionTargetReader`；Connector 始终只返回 SourceItem，`CapturePolicy` 在 application 写入前统一构造 payload version、safe SourceItem fields 与 raw disposition，Monitor adapter 只输出 Source 所需的 immutable 值，不暴露 Monitor record 或 draft。

**Files：** Create `internal/modules/source/domain/{collection,connector,collection_errors}.go`, `internal/modules/source/domain/{collection,connector,collection_errors}_test.go`, `internal/modules/monitor/infrastructure/postgres/collection_target_reader.go`, `internal/modules/monitor/infrastructure/postgres/collection_target_reader_test.go`; Modify `internal/modules/source/domain/ports.go`, `internal/modules/monitor/domain/ports.go`, `tests/architecture/platform_identity_boundary_test.go`.

- [ ] **RED：** 覆盖 Fetch request 必填 window/limit、SourceItem stable external ID、CapturedItem body/metrics 脱敏和 version、raw disposition、错误分类、target 与 published config 归属、draft/paused/disabled source 排除，以及 source 不直接查询 Monitor 表的架构断言。
- [ ] **运行 RED：** `go test ./internal/modules/source/domain ./internal/modules/monitor/infrastructure/postgres ./tests/architecture -run 'TestCollection|TestPublishedCollection|TestSource' -count=1` 必须因契约或 adapter 缺失失败。
- [ ] **GREEN：** 定义纯 domain values/ports；Monitor PostgreSQL adapter 只读取 active published Monitor、published config、enabled MonitorSource 和安全的 SourceConnection ID/签名/词项/locale/interval/checkpoint，不创建或更新任何事实。
- [ ] **重构：** 让 Source application 依赖自己的 port interface，Bootstrap 才连接 Monitor adapter；禁止 Source infrastructure 导入 Monitor PostgreSQL record。
- [ ] **回归：** `go test -race ./internal/modules/source/domain ./internal/modules/monitor/infrastructure/postgres ./tests/architecture -count=1`。
- [ ] **提交：** `git add internal/modules/source/domain internal/modules/monitor/domain internal/modules/monitor/infrastructure/postgres tests/architecture && git commit -m "feat: define collection source contracts"`。

## Task 3：稳定查询规划与 shared request 归并

**目标：** 从 immutable published target 生成可复现 query，按已签名四元组归并，而不重新解释 draft 或建立 QueryPlan 表。

**Consumes：** Task 2 target port 与 Design-014 已保存的 `query_signature`、query override、规则词项、语言和地区。

**Produces：** `QueryPlanner.Plan` 与 `GroupRequests`：相同 source/signature/window 只输出一个 request，target IDs 保留排序；override 优先，空有效词项拒绝；语言/地区/窗口均不改变签名。

**Files：** Create `internal/modules/source/application/query_planner.go`, `internal/modules/source/application/query_planner_test.go`; Modify `internal/modules/source/domain/collection.go`.

- [ ] **RED：** 添加相同 signature 归并、不同 source/window 不归并、排序稳定、override、空词项、paused/draft 排除和 target 不可变 config ID 的测试。
- [ ] **运行 RED：** `go test ./internal/modules/source/application -run TestQueryPlanner -count=1` 必须因 planner 缺失失败。
- [ ] **GREEN：** 实现纯函数 planner，只消费 Task 2 port 返回的值；使用 canonical signature 作为身份，不生成持久化 QueryPlan、动态 DSL 或外部请求。
- [ ] **重构：** 将 query token 规范化与 request grouping 分开，错误保持 typed/可映射，不在日志包含规则全文。
- [ ] **回归：** `go test -race ./internal/modules/source/application -run TestQueryPlanner -count=1`、`go vet ./internal/modules/source/application`。
- [ ] **提交：** `git add internal/modules/source/application/query_planner.go internal/modules/source/application/query_planner_test.go internal/modules/source/domain/collection.go && git commit -m "feat: plan shared collection requests"`。

## Task 4：RSS/Atom 条件请求 Connector

**目标：** 通过固定、可测试的 HTTPS Client 获取 RSS/Atom，分类 HTTP/解析错误并返回统一 SourceItem。

**Consumes：** Task 2 Connector contract、SourceConnection 的 SSRF/allowlist 安全配置。

**Produces：** RSS 与 Atom `Fetch` 支持 ETag、Last-Modified、`If-None-Match`、`If-Modified-Since`、2xx/304、分页链接上限、超时、Retry-After 和脱敏 diagnostics。

**Files：** Create `internal/modules/source/infrastructure/rss/connector.go`, `internal/modules/source/infrastructure/rss/connector_test.go`, `internal/modules/source/infrastructure/rss/feed.go`, `internal/modules/source/infrastructure/rss/feed_test.go`, `internal/modules/source/infrastructure/rss/testdata/{rss,atom}/*.xml`; Modify `internal/modules/source/domain/connector.go`.

- [ ] **RED：** 以 `httptest` 覆盖 RSS、Atom、304、429 Retry-After、5xx、超时、非法 XML、缺 external ID、重复链接和带 credential-shaped redirect；实现前测试必须失败。
- [ ] **运行 RED：** `go test ./internal/modules/source/infrastructure/rss -count=1`。
- [ ] **GREEN：** 实现仅 HTTPS 的固定 client，禁止跨 host/私网 redirect；只解析为 SourceItem，不持久化原始 response bytes、不写 Content、不记录 Authorization/header。Source config 的 CapturePolicy 由 Task 6 统一应用。
- [ ] **重构：** 共享 RSS/Atom 时间、URL、external ID 正规化帮助函数，保持协议结构与 Connector 业务错误分离。
- [ ] **回归：** `go test -race ./internal/modules/source/infrastructure/rss -count=1`。
- [ ] **提交：** `git add internal/modules/source/infrastructure/rss internal/modules/source/domain/connector.go && git commit -m "feat: collect RSS and Atom feeds"`。

## Task 5：Hacker News 增量 Connector

**目标：** 使用官方 HN API 的 ID 增量和有界并发获取 SourceItem，正确处理游标、空结果、限流与临时失败。

**Consumes：** Task 2 Connector contract、官方 `HackerNewsEndpoint` 常量和 Task 3 request window。

**Produces：** HN `Fetch` 返回单调 next cursor、文章/评论安全字段、可分类错误与 server-time diagnostics；不绕过官方 API、不抓取网页。

**Files：** Create `internal/modules/source/infrastructure/hackernews/connector.go`, `internal/modules/source/infrastructure/hackernews/connector_test.go`, `internal/modules/source/infrastructure/hackernews/client.go`, `internal/modules/source/infrastructure/hackernews/testdata/*.json`; Modify `internal/modules/source/domain/connector.go`.

- [ ] **RED：** 使用 `httptest` 覆盖 first/newest ID、已见 cursor、空增量、缺失 item、429、5xx、timeout、乱序 JSON 和最大 item/page 上限。
- [ ] **运行 RED：** `go test ./internal/modules/source/infrastructure/hackernews -count=1`。
- [ ] **GREEN：** 实现官方 endpoint 校验、有限 worker、context 取消、稳定 ID cursor 与 SourceItem 映射；单条坏 item 隔离，整页失败不生成 next cursor。
- [ ] **重构：** 将 API DTO 与 SourceItem mapper 分离，禁止 HN client 输出 raw JSON 或写数据库。
- [ ] **回归：** `go test -race ./internal/modules/source/infrastructure/hackernews -count=1`。
- [ ] **提交：** `git add internal/modules/source/infrastructure/hackernews internal/modules/source/domain/connector.go && git commit -m "feat: collect Hacker News items"`。

## Task 6：共享 run、target、captured item 与 fetch checkpoint 持久化

**目标：** 以单一 PostgreSQL 事务创建或复用 shared run，独立保存 target/item 结果，并只在 durable capture 后推进成功 target checkpoint。

**Consumes：** Tasks 1–5 的 schema、domain、planner 和 Connector result。

**Produces：** `CollectionRepository` PostgreSQL adapter、`CapturePolicy` mapper 与 `CollectionService.Collect`：同四元组只执行一个 fetch；Fetch 后由统一 policy 按 SourceConnection config 将每个 SourceItem 幂等映射为 versioned CapturedItem；每个 target/item 形成可恢复 reconciliation；任一 target capture 失败不影响其他 target；429/5xx/timeout 按 retry state 保存；PLAN-007 可从 item/target-item 读取安全捕获事实而不重新抓取。

**Files：** Create `internal/modules/source/infrastructure/postgres/{collection_record,collection_repository,collection_repository_integration_test}.go`, `internal/modules/source/application/{collection_service,collection_service_test,collection_service_integration_test}.go`; Modify `internal/modules/source/domain/ports.go`, `internal/modules/source/application/service.go`.

- [ ] **RED：** 真实 PostgreSQL 测试覆盖 create-or-reuse race、两个 target 共用一次 fake Connector、由同一 CapturePolicy 对 RSS/HN SourceItem 产生相同的 body/metrics 脱敏与 raw disposition、captured item replay、每个 target-item 对账、item retry 幂等、successful/failed target checkpoint、304/no-content、429 Retry-After、auth/permanent failure、transaction rollback 和 restart from persisted cursor。
- [ ] **运行 RED：** `HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:5432/hotkey_test?sslmode=disable' go test ./internal/modules/source/application ./internal/modules/source/infrastructure/postgres -run 'TestCollection|TestRun' -count=1`。
- [ ] **GREEN：** 由 Source application 在 Fetch 返回后、写入前调用唯一 `CapturePolicy`，再使用 `database.Runtime.WithinTransaction` 保存 run/target/item/target-item/checkpoint；fetch 在事务外执行且以 run status/lock 防重，写入回到单一事务。只在每个 item durable capture 和 target-item reconciliation 完成后推进该 target 的 fetch cursor；不创建 Content、MinIO object、River Job 或跨模块 SQL。
- [ ] **重构：** 分离 Connector registry、run lock、item writer 和 error classifier；Source repository 仅访问 source-owned collection tables，Monitor 数据只来自 Task 2 port。
- [ ] **回归：** `HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:5432/hotkey_test?sslmode=disable' go test -race ./internal/modules/source/... -count=1`。
- [ ] **提交：** `git add internal/modules/source/application internal/modules/source/infrastructure/postgres internal/modules/source/domain && git commit -m "feat: persist shared collection runs"`。

## Task 7：管理员运行 API、健康探测与可观测性

**目标：** 为管理员提供安全的 run 查询、显式重试和来源 health probe，并暴露不含秘密的 collection 指标。

**Consumes：** Task 6 service、identity role middleware、Design-014 Source DTO 安全边界。

**Produces：** `GET /api/v1/collection-runs`、`POST /api/v1/collection-runs/{id}/retry`、`POST /api/v1/source-connections/{id}/health`；viewer/editor 拒绝写操作，安全 DTO 只返回 run/target 状态、计数、时间、错误码和 health；新增固定 `40004` collection run not found、`40005` collection run conflict、`40006` invalid collection request。

**Files：** Modify `docs/design/014-监控配置发布与预览设计.md`, `internal/shared/errors/error.go`, `internal/shared/errors/error_test.go`, `internal/bootstrap/app.go`, `internal/platform/http/router.go`, `docs/openapi/swagger.json`, `tests/architecture/openapi_test.go`; Create `internal/modules/source/transport/http/{collection_dto,collection_handler,collection_routes,collection_handler_test,collection_handler_integration_test}.go`, `internal/modules/source/application/collection_metrics.go`.

- [ ] **RED：** 添加 Handler/HTTP 集成测试，覆盖 admin 成功、viewer/editor 403、未认证 401、无 run 404、冲突 retry 409、invalid request 400、Result `code`、无 secret JSON、OpenAPI route/response 和 `/metrics` collection counter。
- [ ] **运行 RED：** `go test ./internal/modules/source/transport/http ./tests/architecture -run 'TestCollection|TestOpenAPIContract' -count=1`。
- [ ] **GREEN：** 先更新 Design-014 错误码范围，再注册错误码；Handler 仅调用 application service，不暴露 endpoint/config/credential/raw source errors，retry 不启动 Cron/River。
- [ ] **重构：** 抽取 safe run DTO 与 role check，确保 transport 不导入 PostgreSQL adapter，metrics 不含 source ID 或 query 文本标签。
- [ ] **回归：** `HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:5432/hotkey_test?sslmode=disable' HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' go test -race ./internal/modules/source/transport/http ./tests/architecture -count=1`、`make openapi-check`。
- [ ] **提交：** `git add docs/design/014-监控配置发布与预览设计.md internal/shared/errors internal/modules/source/transport/http internal/modules/source/application/collection_metrics.go internal/bootstrap/app.go internal/platform/http/router.go docs/openapi/swagger.json tests/architecture && git commit -m "feat: expose collection run administration"`。

## Task 8：受控验收、独立复核与归档

**目标：** 以真实 PostgreSQL/Redis、fixture Connector 和运行时 HTTP 证据证明采集可用，形成 Acceptance-006 后才归档。

**Files：** Create `docs/acceptance/006-查询规划与RSS-HN采集验收.md`; Modify `docs/acceptance/README.md`, `docs/prd/006-查询规划与RSS-HN采集.md`, `docs/plans/006-查询规划与RSS-HN采集计划.md`, `docs/prd/README.md`, `docs/plans/README.md`, `docs/README.md`, `README.md`.

- [ ] **RED：** 保存 Tasks 1–7 的实际缺失字段、失败 Connector、裸 retry 或权限拒绝信号；不得事后伪造红灯。
- [ ] **GREEN：** 在可丢弃服务运行 `HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:5432/hotkey_test?sslmode=disable' HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' make ci`；以 fixture service 发起 admin HTTP run/health/retry，确认 status、`code:0`、权限、ETag/429/失败 target 和 checkpoint 证据；随后 `make clean` 与 `git diff --check`。
- [ ] **独立复核：** 非主要编写者复核全部 006 提交、schema/records、Monitor→Source 边界、Connector fixture、事务/竞态、HTTP/OpenAPI、日志脱敏、Acceptance 与工作树；Critical/Important 发现必须修复并重跑。
- [ ] **归档：** 复核通过后创建 accepted Acceptance-006，PRD/Plan-006 改为 `archived/done`，索引同步；PLAN-007 才可进入 ready 候选。
- [ ] **提交：** `git add docs && git commit -m "docs: archive collection plan"`。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| Domain/planner | `go test ./internal/modules/source/domain ./internal/modules/source/application -count=1` | 稳定 SourceItem、query request、分组和错误分类通过 |
| Connector | `go test ./internal/modules/source/infrastructure/rss ./internal/modules/source/infrastructure/hackernews -count=1` | RSS/Atom/HN fixture、条件请求、cursor、429/5xx/timeout 通过 |
| 集成 | `HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:5432/hotkey_test?sslmode=disable' go test -race ./internal/modules/source/... -count=1` | shared run、target 隔离、item capture 和 checkpoint 通过 |
| HTTP/OpenAPI | `HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:5432/hotkey_test?sslmode=disable' HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' go test ./internal/modules/source/transport/http ./tests/architecture -count=1` | Result、权限、错误码、脱敏和 OpenAPI 一致 |
| 全量 | `HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:5432/hotkey_test?sslmode=disable' HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' make ci` | 全部质量门禁通过 |

## 风险与回滚

- Content/MinIO/River 仍属于 PLAN-007/013；任何将其塞入 Connector 或 capture service 的改动必须停止并更新 Design/PRD。
- 实际来源协议、DNS/redirect 安全、配额或 API 语义与 Fixture 不一致时保持 Plan in_progress/blocked，先修复 Connector 根因；不得通过关闭校验或抓取网页规避。
- Schema 仍是单一 greenfield `db/schema.sql`。下游未开工前可回退本任务的完整提交；下游开工后仅用新的前向修复 Plan。
- 归档后发现采集回归时保留 Acceptance-006，创建修复 Plan，不恢复旧双轨或重写证据。

## 独立审核

- 2026-07-16：非主要编写者已审核 PLAN-006 v1.7 及其 Design-003/005/012、PRD-006 关联；无 Critical、Important 或 Minor。批准进入 `accepted/ready`，后续每个 Task 仍须测试先行与任务级复核。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-16 | 初始查询规划与 RSS/HN 采集计划。 |
| v1.1 | 2026-07-16 | 对齐 Design-014 的 immutable published config、shared run/target、checkpoint 和来源安全契约；计划仍待完整独立审核。 |
| v1.2 | 2026-07-16 | 将 PLAN-018 治理归档加入前置条件；完整 Task 拆解仍由 PLAN-018 执行，当前保持 review/backlog/pending。 |
| v1.3 | 2026-07-16 | 按产品主链优先级移除 PLAN-018 前置；完整 Task 拆解改由本 Plan 直接完成。 |
| v1.4 | 2026-07-16 | 依据已接受 Design-005/012/014 重写为八个文件级产品 Task；等待独立 Plan Review。 |
| v1.5 | 2026-07-16 | 按独立评审补齐 captured item、raw disposition 与 target-item 对账，确保 PLAN-007 不重新抓取即可恢复。 |
| v1.6 | 2026-07-16 | 按独立评审将 CapturedItem 脱敏/版本策略集中到 Source application，Connector 保持只输出 SourceItem。 |
| v1.7 | 2026-07-16 | 按独立评审把 Design-003 的 Schema/生命周期事实纳入输入、Task 1 文件范围与提交边界。 |
| v1.8 | 2026-07-16 | 独立审核通过，切换为 accepted/approved/ready。 |
