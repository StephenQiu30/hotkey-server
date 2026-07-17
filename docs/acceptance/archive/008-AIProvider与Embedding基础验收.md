---
layer: Acceptance
doc_no: "008"
audience: [Dev, QA, Ops]
feature_area: AI运行基础
purpose: 保存 PLAN-008 AI Provider、运行、预算和向量隔离的长期验收证据
canonical_path: docs/acceptance/archive/008-AIProvider与Embedding基础验收.md
status: accepted
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/archive/008-AIProvider与Embedding基础.md
  - docs/plans/archive/008-AIProvider与Embedding基础计划.md
  - docs/design/archive/003-数据库与数据生命周期设计.md
  - docs/design/archive/007-多语言匹配与相关性设计.md
  - docs/design/archive/011-AI任务证据与模型运行设计.md
  - docs/operations/003-AIProvider与Embedding升级.md
outputs:
  - PLAN-008 验收结论与可复现证据
triggers:
  - PLAN-008 Task 1–7 完成或回归结论变化
downstream:
  - docs/prd/archive/008-AIProvider与Embedding基础.md
  - docs/plans/archive/008-AIProvider与Embedding基础计划.md
result: accepted
---

# AI Provider 与 Embedding 基础验收

## 验收结论与提交范围

PLAN-008 的实现基线为 `fafede8`，独立复核范围为 `b46f73b..fafede8`。真实 disposable fixture、定向负向测试和最终 GREEN 门禁均已通过；非主要编写者已给出 `APPROVED`，因此本文件结论为 `accepted`，PRD/Plan 可归档为 `archived/done`。

| 能力 | 提交 |
|---|---|
| 配置 Schema、catalog、升级/回退 | `b46f73b` |
| Provider-neutral 合同、运行/预算/向量持久化 | `6f35c57`、`0910c27`、`34f315e` |
| OpenAI/ONNX adapter 与计量契约 | `0dcd6b8`、`3fffe76` |
| 运行编排、精确 Embedding provenance | `f19091e`、`2aad0d1` |
| 管理员 model profile API 与 OpenAPI | `fafede8` |

## 验收环境与禁止项

- PostgreSQL/Redis fixture：`HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable'`、`HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15'`。该库和 Redis DB 仅用于可丢弃验收，不记录真实 DSN、API key、dump 路径或本机绝对路径。
- 历史升级/回退固定使用提交 `53d7f01` 的 detached worktree verifier；它不依赖当前工作树的 schema 作为历史正确性来源。
- OpenAI 仅使用 `httptest` fixture 和 dummy key。没有发起真实 Provider 请求，也没有把密钥、Prompt、raw response、endpoint 或 object key 写入证据。

## 受控负向证据

以下均由保留的自动化测试产生，未通过临时破坏生产实现制造回归：

| 风险边界 | 命名测试与结果 |
|---|---|
| 无配置/无密钥安全降级与 exact reuse | `TestEmbeddingServiceDegradesWithoutProviderAndReusesExactRunVector` PASS |
| 同 reuse key 并发仅一个 Provider owner | `TestRunServiceConcurrentIdenticalRequestKeepsSingleProviderOwner` PASS；第二调用得到 `70007 ai_run_in_progress` |
| 预算不足、overage 与 UTC 日封账 | `TestRepositoryRecordsActualOverageAndBlocksTheProfileDay`、`TestRunRepositoryBlocksOverageForUnlimitedBudgetButAllowsNextUTCDay` PASS |
| crash/过期租约回收 | `TestRunRepositoryRefreshesRetryLeaseAndReclaimsOnlyExpiredRuns`、`TestRunRepositoryReclaimsQueuedRunningAndRetryWaitAfterProcessDeath` PASS |
| 非 1024/非有限向量及旧模型隔离 | `TestEmbeddingRepositoryRefusesWrongDimensionsAndNonFiniteValues`、`TestEmbeddingRepositoryFiltersCurrentActiveModelAndUsesHNSW` PASS |
| Provider 错误、JSON/repair 约束 | `TestOpenAIProviderMapsTransportFailuresWithoutLeakingProviderBodies`、`TestOpenAIProviderStructuredRequestUsesStrictSchemaAndBoundedRepair` PASS |
| 管理员、write-only、stale version | `TestModelProfileRoutesEnforceAdminControlPlaneAndRedactCredentials`、`TestModelProfileHTTPPostgresLifecyclePreservesWriteOnlyCredentialBoundary` PASS：未认证 401、非管理员 403、语义 PATCH `credential_ref` 为 70000、stale update 为 409 |

## 真实依赖演练

固定历史 worktree 的 PLAN-007 初始化、备份、PLAN-008 精确升级、当前 `db verify`、未准备 restore 失败、准备后 `pg_restore`、历史 verifier 的链路已由下列命令覆盖并通过：

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
go test -tags=integration ./internal/platform/database \
  -run '^TestPlan008SchemaUpgradeAndRollbackUsesPinnedPlan007Worktree$' -count=1 -v
```

四类 target 的原子 deactivate/insert、只检索 active 的 profile/model-version 向量以及每个 active HNSW index 的 `EXPLAIN (COSTS OFF)` 已由下列测试通过：

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' \
go test -tags=integration ./internal/modules/intelligence/infrastructure/postgres \
  -run '^(TestEmbeddingRepositoryUsesFilteredHNSWForEveryTarget|TestEmbeddingRepositoryFiltersCurrentActiveModelAndUsesHNSW|TestEmbeddingRepositoryRefusesWrongDimensionsAndNonFiniteValues)$' \
  -count=1 -v
```

## 已完成的 GREEN 证据

应用/HTTP/Provider fixture 已在上述 disposable 环境通过，覆盖同 key 互斥、预算不可用时回退、exact provenance reuse、管理员 CRUD/restore、OpenAI SDK `model` 精确等于 profile `model_name`、local `model_version` 不进入 SDK 请求、429/5xx/deadline/非法 JSON 的安全 numeric code 映射及一次受限 repair：

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
go test -tags=integration ./internal/modules/intelligence/application \
  ./internal/modules/intelligence/transport/http -count=1

go test ./internal/modules/intelligence/infrastructure/provider \
  -run '^(TestOpenAIProviderEmbedUsesModelNameAndReturnsLocalVersion|TestOpenAIProviderMapsTransportFailuresWithoutLeakingProviderBodies|TestOpenAIProviderStructuredRequestUsesStrictSchemaAndBoundedRepair)$' \
  -count=1

HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
CGO_ENABLED=0 go test -tags=onnx ./... -count=1
```

代码基线完成后及 Task 7 文档收口后，完整 `make ci` 均已通过（包含 Swagger、vet、build、全量测试、schema/architecture/repository 校验与数据库 runtime verify）；下列最终门禁亦已通过并纳入归档提交前检查：

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
make ci

HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' \
go test -race -tags=integration ./internal/modules/intelligence/... \
  ./internal/platform/database/... ./test/architecture -count=1

make clean
git diff --check
git status --short
```

## 残余风险

默认构建和 `CGO_ENABLED=0 -tags=onnx` 均已全树通过，缺少 native runtime/model/tokenizer/manifest 时会安全降级。受控主机上的完整签名 `onnx && cgo` bundle 推理矩阵尚未执行：当前没有可审计的 native runtime、模型、tokenizer 与 manifest hash 组合。因此它保留为上线前的显式运行环境验收项，不能被描述为已经通过，也不影响默认安全降级契约。

## 独立最终复核与发布决定

非主要编写者已审阅实现、Schema/record/catalog、历史升级/回退、SDK fixture、ONNX 默认降级、JSON Schema、并发锁/预算、向量 HNSW、API/OpenAPI、日志/指标脱敏、完整命令及最终 worktree，结论为 `APPROVED`，没有未解决 Critical/Important 问题。PRD/Plan 与索引已同步为 `archived/done`；PLAN-009 仍须独立满足自己的 accepted/approved/ready 门禁。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-17 | 创建实施前验收模板，固定 Provider fixture、预算/并发、向量/HNSW、upgrade/rollback、HTTP/OpenAPI 和独立复核证据；尚未执行。 |
| v0.2 | 2026-07-17 | 补齐 task-type 拒绝、max/daily/overage、lease recovery、ONNX bundle 和固定 historical verifier 证据；尚未执行。 |
| v0.3 | 2026-07-17 | 补齐 overage 封账、重试 lease 刷新、统一锁序与无 CGO ONNX 安全降级证据；尚未执行。 |
| v0.4 | 2026-07-17 | 增加 create-only credential_ref 与 OpenAI model ID/local version 边界的验收证据；尚未执行。 |
| v0.5 | 2026-07-17 | 记录 `b46f73b..fafede8` 的真实 fixture、负向测试、升级/HNSW/SDK/ONNX GREEN 证据；等待独立最终复核与最终门禁。 |
| v0.6 | 2026-07-17 | 按独立复核补齐 ONNX 全树命令的 PostgreSQL/Redis fixture，并重新执行该矩阵；仍待最终复核。 |
| v1.0 | 2026-07-17 | 最终 make ci、integration race 和带 fixture 的 ONNX 全树矩阵通过；独立最终复核 APPROVED，验收结论 accepted。 |
