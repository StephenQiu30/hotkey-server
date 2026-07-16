---
layer: Acceptance
doc_no: "006"
audience: [Dev, QA, Ops]
feature_area: 来源采集
purpose: 保存查询规划、RSS/Atom 与 Hacker News 共享捕获运行的受控验收证据
canonical_path: docs/acceptance/006-查询规划与RSS-HN采集验收.md
status: accepted
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/006-查询规划与RSS-HN采集.md
  - docs/plans/006-查询规划与RSS-HN采集计划.md
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/005-数据来源查询规划与采集设计.md
  - docs/design/012-监控调度与River流水线设计.md
  - docs/design/014-监控配置发布与预览设计.md
outputs:
  - PLAN-006 验收结论与可复现证据
triggers:
  - PLAN-006 Task 1–8 完成或回归结论变化
downstream:
  - docs/prd/006-查询规划与RSS-HN采集.md
  - docs/plans/006-查询规划与RSS-HN采集计划.md
result: accepted
---

# 查询规划与 RSS/HN 采集验收

## 结论与提交范围

PLAN-006 的发布验收为 `accepted`。实现证据基线为 `b905b59`，覆盖从 `8cf8d56` 到 `b905b59` 的实现和任务级复核；最终独立全链复核无 Critical、Important 或 Minor，使用本文件定义的可丢弃 fixture 环境运行完整 `make ci` 通过，并在完成后执行 `make clean` 与 `git diff --check`。

| Task | 已审核提交 | 已保存的任务级结论 |
|---|---|---|
| 1 Schema/records | `8cf8d56`、`2d0059b`、`8be4f9b` | 通过：完整 Schema、record 与同 run target-item 复合外键。 |
| 2 contracts/boundary | `9e21861`、`c9cf04c`、`ef1892a` | 通过：Connector 契约与 Monitor→Source 窄读取边界。 |
| 3 planner | `f1d8bd3`、`6c55aa4`、`289d412` | 通过：immutable target 回推校验与 shared request 归并。 |
| 4 RSS/Atom | `a04bcb0`、`57e29a7`、`2cc15d4`、`71e219c` | 通过：fixture 条件请求、continuation 与 redirect 边界。 |
| 5 Hacker News | `f8db02c`、`6794c4a`、`c114449` | 通过：high-watermark、限流与取消分类。 |
| 6 durable run | `f611f00`、`5b53888`、`4bef47b` | 通过：共享 run、目标隔离、可重领与持久化 checkpoint。 |
| 7 admin/control | `d157d75`、`b905b59` | 通过：安全 run/health/retry API、指标与 retry reconciliation。 |

## 验收环境与边界

- 任务级 PostgreSQL 集成测试使用可丢弃库 `HOTKEY_TEST_DSN='postgres:///hotkey_plan006_test?sslmode=disable'`；测试 fixture 会新建并清理其数据库，未连接生产库或真实来源。该 local-socket role 具备 fixture 所需的创建/删除数据库权限。
- 全量质量门禁使用可丢弃 PostgreSQL 和 Redis DB 15：`HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15'`。Redis 仅承载既有身份回归状态，collection 的业务事实均在 PostgreSQL。
- RSS/Atom/HN 只使用本地 `httptest` fixture；HTTP 管理端到端测试使用真实 PostgreSQL、Gin 路由和 fixture connector，而非真实凭据、互联网来源或浏览器。
- 本验收不把 Content 标准化/去重、MinIO、River、Cron、AI、事件、报告或投递描述为 006 已交付；它们分别属于后续 PLAN-007、PLAN-013 及其后的产品任务。

## 已发生的受控 RED 与修复基线

下表只转录各 Task 在实现或复核时已运行过的实际失败信号，不在验收阶段重新制造或伪造红灯。

| 范围 | RED 命令或测试 | 实际失败信号 | 最终修复基线 |
|---|---|---|---|
| Schema/records | `go test ./internal/platform/database/model ./internal/platform/database ./tests/architecture -run 'TestCollection\|TestCompleteSchemaCoversMappedRecords\|TestGreenfieldSchemaEnforcesCriticalConstraints' -count=1` | 旧 Schema 缺少 capture 字段、唯一键或同 run target-item 约束。 | `2d0059b` 用复合外键拒绝跨 run target-item 对账。 |
| Source 契约 | `go test ./internal/modules/source/domain ./internal/modules/monitor/infrastructure/postgres ./tests/architecture -run 'TestCollection\|TestPublishedCollection\|TestSource' -count=1` | disabled/deleted SourceConnection 被错误列为可采集 target。 | `c9cf04c` 收紧可用来源资格谓词。 |
| 规划归并 | `go test ./internal/modules/source/application -run TestQueryPlannerGroupRequestsRejectsDriftFromPublishedTarget -count=1` | query、语言或地区被篡改后仍返回 nil。 | `6c55aa4` 按 immutable target 回推并拒绝漂移。 |
| RSS/Atom | connector fixture 复核测试 | 外部 host redirect/cursor continuation 未受 immutable endpoint host 限制，且 continuation 可污染根 feed validators。 | `57e29a7`，并由 `2cc15d4` 补齐 credential-shaped redirect 覆盖。 |
| Hacker News | connector fixture 复核测试 | `maxItems` 后 parent cancellation 可把未抓取 ID 记为已处理；并发 429 可被 cancellation temporary 错误掩盖。 | `6794c4a` 保留完整范围与原始 rate-limit 分类。 |
| durable run | `go test ./internal/modules/source/application ./internal/modules/source/infrastructure/postgres -run 'TestCollection\|TestRun' -count=1` | queued/stale-running run 无法重领；不同 checkpoint target 错继承 cursor/ETag。 | `5b53888` 只重领 queued 或 stale-running，并按等价 checkpoint 状态隔离。 |
| admin retry | `go test ./internal/modules/source/transport/http -run TestCollectionAdminRoutes -count=1`；`TestCollectionRepositoryRetryRepairsCheckpointConflictTargetReconciliation` | 初始缺失 control type/route；冲突后的成功重试仍保留 `outcome="failed" reason="checkpoint_conflict"`。 | `d157d75` 定义 API；成功对账覆盖 failed outcome 并清空 reason。 |

## 已完成的 GREEN 证据

- 真正 PostgreSQL 的 `TestCollectionServiceFetchesOnceAndDurablyReconcilesEveryTarget` 证明同一四元组只执行一次 fixture connector 请求、每个 immutable target 都有对账，且成功 checkpoint 保存 cursor、ETag 和 run ID；captured payload 不含原始 authorization、原始 body 或 transient 内容。
- `TestCollectionServiceFailureRetainsCursorAndPersistsRetryState` 证明 429 的 `Retry-After` 持久化到 run/checkpoint、原 cursor 不倒退；`TestCollectionServiceIsolatesOneTargetCheckpointConflict` 证明一个 target 冲突时另一 target 仍可成功推进。
- RSS/Atom 与 HN 的独立 fixture 测试覆盖条件请求、cursor/ID 增量、429、5xx、超时与取消分类；没有真实来源请求。
- `TestCollectionAdminHTTPIntegrationUsesSafeDTOsAndDurableStateCommands` 以真实 PostgreSQL、Gin router 和 fixture connector 发起 `GET /api/v1/collection-runs`、`POST /api/v1/collection-runs/{id}/retry`、`POST /api/v1/source-connections/{id}/health`：列表返回 `code: 0` 的 failed safe run，retry 返回 queued run，health 持久化 degraded 状态；source ID、query signature、cursor、ETag、endpoint、credential 和 fixture secret 均不出现于 JSON。
- `TestCollectionAdminRoutesEnforceRolesAndExposeOnlySafeRunFacts` 验证 admin 三个操作均为 HTTP 200 / `code: 0`，viewer list 与 editor retry 为 403；`TestCollectionAdminRoutesReturnStableRunErrorsAndRejectInvalidInput` 验证未认证 401、run 不存在 404/40004、fresh run 409/40005、非法 query 400/40006。
- Task 7 基线已在可丢弃 PostgreSQL/Redis 环境通过 `go test -race ./internal/modules/source/... ./tests/architecture -count=1`、`make openapi-validate` 与完整 `make ci`，并在完成后运行 `make clean` 和 `git diff --check`。

## 最终质量门禁

下列命令已在可丢弃 fixture 环境通过。维护 DSN 必须有创建、删除临时数据库的权限；当前 local-socket DSN 满足该条件。

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_plan006_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
make ci
make clean
git diff --check
git status --short
```

独立复核已覆盖全部 006 提交、Schema 与 record 一致性、Monitor→Source 所有权、RSS/Atom/HN fixture、事务/竞态、run/retry 状态机、HTTP/OpenAPI/Result、日志与 DTO 脱敏、指标标签、本文档和工作树。原有的不可复现 host DSN 已在验收前改为与已验证 fixture 一致的维护 DSN；整改后复核通过。

## 残余风险

真实来源协议、DNS/redirect 安全、配额和 API 语义仍可能与 fixture 有差异；出现差异时以新的前向修复 Plan 关闭，而不是绕过校验或以网页抓取替代 Connector。Content 标准化、去重与 MinIO 仍是 PLAN-007，Cron/River 是 PLAN-013；本验收不替代其各自门禁。

## 独立最终复核

- 非主要编写者复核 `8cf8d56..b905b59`、Acceptance-006 与工作树。初次发现最终门禁 DSN 与实际 fixture 环境不一致（Important）；修正为具建删库权限的 local-socket DSN 后，`make validate`、`git diff --check` 与相同 DSN + Redis/15 的完整 `make ci` 均通过。
- 复核确认 Schema/records、Monitor→Source 边界、RSS/Atom/HN 安全与错误语义、事务/重领/checkpoint/retry reconciliation、HTTP/OpenAPI/DTO/权限和低基数指标均无 Critical、Important 或 Minor，结论为 `APPROVED`。

## 发布决定

允许将 PRD/Plan-006 标记为 `archived/done`。PLAN-007 因此前置完成成为进入自身独立审核后的 `ready` 候选；其当前 `review/backlog/pending` 状态不因此自动改变。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-16 | 汇总 Task 1–7 的真实 RED/GREEN、fixture HTTP、范围边界和最终复核门禁；结论保持 review。 |
| v1.0 | 2026-07-16 | 修正为可复现的 fixture 维护 DSN，完整质量门禁与独立最终复核通过；结论改为 accepted。 |
