---
layer: Acceptance
doc_no: "008"
audience: [Dev, QA, Ops]
feature_area: AI运行基础
purpose: 保存 PLAN-008 AI Provider、运行、预算和向量隔离的长期验收证据
canonical_path: docs/acceptance/008-AIProvider与Embedding基础验收.md
status: review
version: v0.1
owner: HotKey Server Team
inputs:
  - docs/prd/008-AIProvider与Embedding基础.md
  - docs/plans/008-AIProvider与Embedding基础计划.md
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/007-多语言匹配与相关性设计.md
  - docs/design/011-AI任务证据与模型运行设计.md
  - docs/operations/plan008-schema-upgrade.md
outputs:
  - PLAN-008 验收结论与可复现证据
triggers:
  - PLAN-008 Task 1–7 完成或回归结论变化
downstream:
  - docs/prd/008-AIProvider与Embedding基础.md
  - docs/plans/008-AIProvider与Embedding基础计划.md
result: pending
---

# AI Provider 与 Embedding 基础验收

## 当前状态

本文件是实施前已审核的验收模板，不表示任何 AI 能力已经交付。只有 PLAN-008 全部任务完成、真实 disposable PostgreSQL/Redis evidence 保存、独立最终复核通过后，才能将 `result` 和 `status` 改为 accepted，并同步 PRD/Plan 与索引状态。

## 必须记录的范围与环境

- 实现基线 commit、审核范围和每个 Task 的提交。
- `HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable'`、`HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15'` 等可丢弃 fixture；不记录真实 DSN、API key、dump 路径或本机绝对路径。
- OpenAI adapter 仅允许 `httptest` fixture 与 dummy key；不得以真实 Provider 调用作为验收证据。
- ONNX default build 必须在无 native library/模型下通过；可选 `onnx` matrix 只有在受控 host 执行时记录 runtime 版本、模型 hash 和结果，未执行必须列为残余风险，不能伪装为通过。

## 必须保留的 RED 证据

实施完成前在测试中可重放以下负向信号；不得为截图临时破坏实现：

1. 不存在可用 profile、默认 build 下 ONNX profile、缺少 OpenAI key 都产生安全降级，ingestion 与 Content read 仍成功。
2. 第二个同 `reuse_key` 并发请求观察到 `70007 ai_run_in_progress`，Provider fixture 只收到一次调用。
3. 单次/日预算耗尽不外呼；失败/取消后 reservation 释放；超额 settlement 失败且不透支。
4. 1023、1025、`NaN`、`Inf` 向量被拒绝；不同 profile/model version 或 inactive 行不出现在近邻结果。
5. 429、5xx、deadline、非法 JSON、第二次修复失败均映射为稳定 numeric code；响应、日志、指标与 OpenAPI 不含 key、credential_ref、Prompt、raw response、endpoint 或 object key。
6. 未认证、非管理员、stale version、语义 profile PATCH 的 HTTP Result 与 OpenAPI 契约都失败且安全。
7. 未执行准备步骤的 legacy `pg_restore` 按手册失败；准备后恢复和 historical PLAN-007 verifier 通过。

## 必须保留的 GREEN 证据

### Provider、Schema、复用和预算

记录命令和命名测试，至少覆盖 OpenAI `httptest` 成功/429/5xx/timeout/invalid JSON/repair、静态 JSON Schema、一次修复、profile 选择、same-key claim、success-only reuse、版本失效和 profile+UTC-day reserve/settle/release 并发。

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' \
go test -race -tags=integration ./internal/modules/intelligence/... -count=1
```

### 向量、HNSW、升级和 HTTP

记录四类 target 的 atomic deactivate/insert、profile+model-version-filtered search 和每个 active HNSW index 的 `EXPLAIN (COSTS OFF)` 摘要。执行 PLAN-007 legacy backup -> PLAN-008 exact upgrade -> current `db verify` -> prepared `pg_restore` -> historical PLAN-007 `db verify` 的真实演练。使用 Gin/管理员 login fixture 验证六个 profile API 路由、角色矩阵、write-only 输出与生成 OpenAPI。

```bash
HOTKEY_TEST_DSN='postgres:///hotkey_plan008_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
make ci
make clean
git diff --check
git status --short
```

## 独立最终复核与发布决定

非主要编写者必须审阅实现、Schema/record/catalog、真实升级/回退、SDK fixture、ONNX 默认降级、JSON Schema、并发锁/预算、向量 HNSW、API/OpenAPI、日志/指标脱敏、完整命令和干净 worktree。未解决 Critical/Important、未执行的必需 fixture 或未解释的 optional ONNX 结果均不得 accepted。

结论 accepted 后才允许将 PRD/Plan-008 改为 `archived/done`，并在 README/PRD/Plan/Acceptance/Operations 索引同步完成状态。PLAN-009 仍须独立满足它自己的 accepted/approved/ready 门禁。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-17 | 创建实施前验收模板，固定 Provider fixture、预算/并发、向量/HNSW、upgrade/rollback、HTTP/OpenAPI 和独立复核证据；尚未执行。 |
