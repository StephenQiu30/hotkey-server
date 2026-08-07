---
layer: Plan
scope: backend
doc_no: "009"
title: Hacker-News官方来源连接器计划
status: completed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/plans/009-Hacker-News官方来源连接器计划.md
design: docs/design/009-Hacker-News官方来源连接器设计.md
prd: docs/prd/009-Hacker-News官方来源连接器.md
---

# Hacker-News官方来源连接器计划

## 前置门禁

- [x] Design 状态改为 `accepted`，PRD 状态改为 `approved`。
- [x] 以 Hacker News 官方 API 仓库核对 item 类型、可选字段、父项和 `maxitem` 语义。
- [x] 对照当前代码保存 ParentExternalID 缺失的编译失败，以及部分窗口重试契约失败测试。

## 实际变更范围

- `backend/internal/modules/source/domain/collection.go`
- `backend/internal/modules/source/infrastructure/hackernews/connector.go`
- `backend/internal/modules/source/infrastructure/postgres/collection_repository.go`
- `backend/test/_suite/internal/modules/source/`
- `docs/design/`、`docs/prd/`、`docs/plans/`、`docs/acceptance/`

## 执行步骤

1. [x] 以失败测试固定五类 item、父项、可选指标、连续前缀、部分成功、全量失败、并发和网络边界。
2. [x] 在 SourceItem、CapturedItem 与向后兼容的 CapturedItem v2 JSON 中增加可选 `parent_external_id`；无需新增表、列或 migration。
3. [x] 映射 `story`/`job`/`poll` 与 `comment`/`pollopt`，保持显式零指标和缺失指标的差异。
4. [x] 将 item 读取限制为 4 worker；临时单项失败继续完成有界窗口，认证、限流和永久错误停止继续派发。
5. [x] 只返回首个未完成 ID 之前的连续前缀，令下一次 checkpoint 从失败位置继续；全量失败保持原 checkpoint。
6. [x] 复用既有 CollectionRun、来源健康、OpenAPI 和来源页面；本条无新增公开契约或前端 Client。
7. [x] 使用 `$agent-browser` 验证 Hacker News 来源的创建、固定端点、探测错误、列表错误恢复、权限、桌面/移动布局与 WCAG A/AA；CollectionRun 状态由应用集成测试覆盖。
8. [x] 新增 `docs/acceptance/009-Hacker-News官方来源连接器验收.md` 并完成全量门禁；本条无新增生产人工操作，不新增 Operations。

## 验证

- `cd backend && make ci`
- `cd backend && go run ./test/runner test -race ./internal/modules/source/infrastructure/hackernews -count=1`
- `cd frontend && npm run openapi:check && npm run typecheck && npm run test:unit && npm run build`
- 从仓库根运行两套 Compose `config --quiet` 与 `git diff --check`。
- 使用 `$agent-browser` 覆盖管理员正常流程、固定端点、上游错误、来源列表错误恢复、权限、桌面、移动、键盘和 axe WCAG A/AA；采集状态由应用集成测试覆盖。

## 迁移与回滚

`parent_external_id` 是 CapturedItem v2 JSON 的可选字段，旧记录缺失时保持空值；不需要 Schema 迁移。回滚应用版本会忽略该 JSON 字段，不删除 CapturedItem、Content、CollectionRun 或 checkpoint。先停用 Hacker News SourceConnection，再恢复上一应用版本即可停止新增采集。

## 依赖与功能切片

- 前置编号：007，已通过验收。

- [x] Slice-009-1：实现官方 API 增量采集和单调 checkpoint。
- [x] Slice-009-2：实现完整 item 类型、父项和证据字段映射。
- [x] Slice-009-3：实现公开指标的存在性快照。
- [x] Slice-009-4：实现有界并发、确定性故障分类、部分成功与未完成窗口重试。

## 完成定义

- 断点续传只重试未完成窗口，连续成功前缀不漏不重。
- 官方端点之外的地址、重定向和非公网解析全部拒绝。
- 部分失败与全量失败在 CollectionRun 状态和计数中可区分。
- 自动化、浏览器和全量回归均通过并形成同编号 Acceptance。
