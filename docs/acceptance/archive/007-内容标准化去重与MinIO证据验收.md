---
layer: Acceptance
doc_no: "007"
audience: [Dev, QA, Ops]
feature_area: 内容与证据
purpose: 保存 Content 标准化、确定性去重、MinIO 证据与删除同步的受控验收证据
canonical_path: docs/acceptance/archive/007-内容标准化去重与MinIO证据验收.md
status: accepted
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/archive/007-内容标准化去重与MinIO证据.md
  - docs/plans/archive/007-内容标准化去重与MinIO证据计划.md
  - docs/design/archive/003-数据库与数据生命周期设计.md
  - docs/design/archive/006-内容标准化去重与证据设计.md
  - docs/operations/plan007-content-normalization-minio-evidence-upgrade.md
outputs:
  - PLAN-007 验收结论与可复现证据
triggers:
  - PLAN-007 Task 1–8 完成或回归结论变化
downstream:
  - docs/prd/archive/007-内容标准化去重与MinIO证据.md
  - docs/plans/archive/007-内容标准化去重与MinIO证据计划.md
result: accepted
---

# 内容标准化、去重与 MinIO 证据验收

## 结论与提交范围

PLAN-007 的发布验收为 `accepted`。实现基线为 `a7d112b`，审核范围覆盖 `b87111b..a7d112b`；真实依赖、恢复演练、Gin 运行态、完整 CI/race 与非主要编写者的最终复核均已通过。本验收不包含后续 PLAN-008 及以上任务。

| 范围 | 已审核提交 | 已保存结论 |
|---|---|---|
| Schema、capture 边界与既有库升级 | `b87111b`、`a7d112b` | nullable metrics、source 复合归属、可验证的 legacy 升级与精确回退。 |
| 标准化与去重 | `8151bd0`、`357d08f` | 只消费 durable capture；exact URL/hash 可跨来源，near-text 仅限同来源。 |
| Content/asset 持久化 | `8ff7f24`、`03e726c`、`69552a3` | 来源幂等、SQL NULL/显式零、资产 URL 边界与冲突安全。 |
| MinIO 与处理一致性 | `486d02a`、`1222c62`、`a058d9f`、`a91be85`、`a53119d` | 确定性对象键、补偿、对账和并发重放。 |
| 删除与安全查询 | `e8e1eca`、`9135402`、`2042f76`、`9bba721` | tombstone 优先、幂等删除、active-only DTO 和来源可用性过滤。 |

## 验收环境与边界

- PostgreSQL、Redis 与 MinIO 均为本地可丢弃 fixture：`HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable'`、`HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15'`，以及 `历史 MinIO fixture 脚本（已移除）` 提供的 `127.0.0.1:19007` / bucket `hotkey-plan007`。fixture 在本验收结束后已执行 `down`；没有连接生产库、真实来源或真实凭据。
- 运行态 HTTP 使用单独的空 PostgreSQL 库、真实 Gin、Redis/15 与同一 disposable MinIO，显式提供长度合格的 JWT/HMAC 密钥。它不读取或修改 `.env`、`.env.prod`。
- 既有库演练由完整 PLAN-006 Schema、custom `pg_dump`、手册中的精确升级与回退 SQL 构成；临时库、dump 和 detached legacy worktree 都在命令退出时清理。
- 本验收只证明 Content/capture/MinIO 边界；不把 Embedding、跨语言语义、事件、AI、Cron/River、知识、报告或投递描述成已交付。

## 已发生的受控 RED 与修复基线

下列信号来自真实 PostgreSQL/MinIO/Gin 路径；本验收没有重新制造已修复的缺失实现或把测试替身写成外部依赖证据。

| 范围 | 实际负向信号 | 修复或当前约束 |
|---|---|---|
| 既有库回退 | 未先删除 PLAN-007 的两个复合外键时，真实 `pg_restore --clean --if-exists --no-owner` 报 `cannot drop table`，并指出 `collection_run_items_content_source_connection_fkey`。 | `a7d112b` 在手册中只删除这两个 PLAN-007 外键，禁止宽泛 reset；integration test 先断言该失败，再验证恢复。 |
| 对象补偿/对账 | 注入 Source bind 失败后，事务不能留下 asset/bind；注入首次对象删除失败后会留下可见 orphan。 | 真实 MinIO `Head`/`ListPrefix` 与 PostgreSQL 断言证明 rollback 无对象、reconcile 仅删除 orphan 并保留已引用对象。 |
| 权限与生命周期 | 真实 Gin 的未认证 content list 为 HTTP 401；deleted Content detail 为 HTTP 404。 | public route 必须认证，查询只返回 active 且可用来源的 allowlist DTO。 |

## 已完成的 GREEN 证据

### MinIO、事务与领域回归

启动 fixture 后，以下 PLAN 要求的精确命令通过：

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' \
HOTKEY_TEST_MINIO_ENDPOINT=127.0.0.1:19007 \
HOTKEY_TEST_MINIO_ACCESS_KEY=hotkey-plan007 \
HOTKEY_TEST_MINIO_SECRET_KEY=hotkey-plan007-secret \
HOTKEY_TEST_MINIO_BUCKET=hotkey-plan007 \
go test -tags=integration ./internal/modules/ingestion/application \
  -run 'TestIngestRun(MinIOPostgresRollbackDeletesObject|MinIOPostgresReconcileDeletesOrphan)' \
  -count=1
```

两个真实 MinIO/PostgreSQL 测试均通过。rollback 路径确认 `content_assets=0`、capture 未绑定、`ListPrefix` 为空且 `Head` 返回对象不存在；reconcile 路径先确认 orphan 可 `Head`，随后仅删除该 orphan，保留已引用对象（`Head` 成功、`ListPrefix` 仅一项、`content_assets=1`）。`TestIngestRunMinIOPostgresRePutsEvidenceDeletedBeforeAssetTransaction` 也通过，证明对账不会错误删除事务内最终应存在的对象。

同一环境中的范围回归通过，覆盖 ingestion、source、database 和 architecture 包。下列命名验证以 `-v` 通过：

- `TestCollectionServiceFetchesOnceAndDurablyReconcilesEveryTarget`：同一请求只 fetch 一次、每个 target durable reconcile，不补抓正文。
- `TestDecideDuplicateNearTextRequiresSameSourceWindowAndStrictSimilarity/cross_source_independent_report` 与 `TestDecideDuplicateKeepsCrossLanguageReportsAndSameSourceRetryIsExact`：跨来源独立报道和跨语言报道不被 near-text 合并；同源 retry 仍精确幂等。
- `TestContentRepositoryUpsertIsSourceIdempotentAndRaceSafe`、`TestContentRepositoryPreservesUnknownAndExplicitZeroMetrics`：source identity 并发安全，`NULL` 与显式 `0` 分别保留。
- `TestDeleteBySourceItemMarksContentDeletedBeforeRetryingEvidenceDeletion` 与 `TestDeleteBySourceItemMissingSourceItemIsIdempotentNoOp`：先 tombstone、后对象补偿，重复删除是 no-op。
- `TestContentRoutesPostgresIntegrationExposeOnlyActiveSafeContent`：真实 PostgreSQL + Gin 路由只返回 active safe Content。

### 既有库升级、验证与回退

在完整 legacy Schema 的可恢复副本上，执行 custom `pg_dump`、`pg_restore --list`、`docs/operations/plan007-content-normalization-minio-evidence-upgrade.md` 的整个 transaction、当前 release `hotkey db verify`、手册中的精确外键清理与 `pg_restore`；最后切换到创建 backup 的 legacy release 再运行其 `hotkey db verify`。真实演练结果为：

```text
backup-list=498
preflight=0|0
upgraded=0|0|0|0|0|0|12|0|4|0|8|0|2|pending,succeeded
current-db-verify=passed
rollback=1|1|0|0|0
legacy-release-db-verify=passed
```

`upgraded` 依次证明错属、null source、非法 ingestion state、legacy zero Content/snapshot 均为零；随后是 Content 的 view/like/comment/share 聚合 `0/12/0/4`、snapshot 的 `0/8/0/2`，以及 `pending,succeeded`。`rollback` 证明 legacy content 和 snapshot 各恢复一条、两个 dedupe 列和 `source_connection_id` 已消失、两个 PLAN-007 外键已消失。此路径既不重新 Fetch 来源，也不使用 `DROP SCHEMA ... CASCADE`。

### 真实 HTTP、OpenAPI 与质量门禁

独立空库上以 `hotkey serve --role api --http-addr 127.0.0.1:19008` 运行真实 Gin，bootstrap admin 登录后得到：`/healthz=200`、未认证 `GET /api/v1/contents=401`、登录=200、认证 list/detail=200、deleted detail=404。list/detail 均只含 DTO allowlist；`body`、`excerpt`、`object_key`、`asset`、MinIO endpoint/credential 等字段不存在。active fixture 的 `view_count=0`，未报告的 `like_count=null`，验证零与未知在 HTTP 边界仍可区分。

以下最终门禁均在同一可丢弃依赖集合通过；`make ci` 已包含 OpenAPI 漂移、空库 Schema/runtime verify、完整测试、构建和架构/仓库校验，随后运行 `make clean`：

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_plan007_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
HOTKEY_TEST_MINIO_ENDPOINT='127.0.0.1:19007' \
HOTKEY_TEST_MINIO_ACCESS_KEY='hotkey-plan007' \
HOTKEY_TEST_MINIO_SECRET_KEY='hotkey-plan007-secret' \
HOTKEY_TEST_MINIO_BUCKET='hotkey-plan007' \
make ci
make clean
```

同一 PostgreSQL/Redis/MinIO fixture 下，`go test -race -tags=integration ./internal/modules/ingestion/... ./internal/modules/source/... ./internal/platform/database/... ./test/architecture -count=1` 通过。验收结束后已执行 `外部 disposable MinIO fixture 已停止`、`git diff --check` 与 `git status --short`；没有 fixture 容器、构建二进制或未提交实现改动。

## 残余风险

真实来源的许可、协议、配额和对象存储线上故障语义仍可能与本地 fixture 不同。遇到差异须以新的前向修复 Plan 关闭；不得重抓来源、放宽 source/Content 归属、暴露对象下载接口，或绕过手册的备份、回退与 verifier release 边界。

## 独立最终复核

非主要编写者已复核 `b87111b..a7d112b`、本 Acceptance、Design/PRD/Plan、Source→Ingestion 边界、Schema/records、MinIO 故障与事务竞态、HTTP/OpenAPI/日志指标脱敏，以及 clean worktree。复核结论为 `APPROVED`，无未解决 Critical 或 Important；若后续发现回归，必须先以新的前向修复关闭，再重新执行受影响的验收命令。

## 发布决定

允许将 PRD/Plan-007 标记为 `archived/done` 并同步任务索引与 README。PLAN-008 仍必须自行满足 `accepted/approved/ready` 门禁；PLAN-007 完成不会自动将它改为 `ready`。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-17 | 汇总真实 PostgreSQL/Redis/MinIO、legacy upgrade/rollback、Gin HTTP、CI/race 与受控 fixture 证据；等待独立最终复核。 |
| v1.0 | 2026-07-17 | 独立最终复核通过，结论改为 accepted，允许归档 PRD/Plan-007。 |
