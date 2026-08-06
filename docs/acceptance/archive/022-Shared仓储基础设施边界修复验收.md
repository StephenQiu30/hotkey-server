---
layer: Acceptance
scope: backend
doc_no: "022"
audience: [Dev, QA, Ops]
feature_area: 工程目录与依赖边界
purpose: 验收 shared 基础设施中立与 GORM/pgx 适配器迁移
canonical_path: docs/acceptance/archive/022-Shared仓储基础设施边界修复验收.md
status: accepted
conclusion: accepted_with_risk
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/archive/022-Shared仓储基础设施边界修复.md
  - docs/plans/archive/022-Shared仓储基础设施边界修复计划.md
outputs:
  - PLAN-022 长期验收结论
triggers:
  - PLAN-022 开始实施
downstream:
  - docs/acceptance/README.md
---

# Shared 仓储基础设施边界修复验收

## RED/GREEN 证据

| 类型 | 命令/证据 | 状态 |
|---|---|---|
| shared 边界 RED | 新门禁准确报告 shared 中 pgx、GORM、platform model 共 4 条违规 import | passed |
| shared 边界 GREEN | `go test ./test/architecture -run TestSharedPackagesDoNotImportInfrastructure -count=1` | passed |
| 错误映射 | 定向覆盖 GORM not-found、全部既有 pgx code、context 取消及 wrapped GORM/pgx 输入 | passed |
| 生产包编译 | `go run ./test/runner test ./internal/modules/... ./internal/platform/... -run '^$' -count=1` | passed |
| 调用点边界 | 29 个文件使用 platform `MapError`；shared 错误仍直接来自 shared；shared 无基础设施 import | passed |
| 工程门禁 | `make lint`、`make build`、`make validate`、`git diff --check`，随后 `make clean` | passed |
| 事实源保护 | `db/schema.sql`、`docs/openapi`、运行时 intelligence schemas 零差异 | passed |
| GORM CRUD 集成 | 未配置隔离 `HOTKEY_TEST_DSN` | blocked |
| 完整测试/CI | 未配置隔离 `HOTKEY_TEST_DSN` 与 `HOTKEY_TEST_REDIS_URL`；`make test` 在 DSN 入口终止 | blocked |
| 独立实现复审 | 非主要实施者复核 79df138；反引号门禁 P2 修复后无新的 P0/P1/P2，结论 `APPROVED` | passed |

## 结论

`accepted_with_risk`：shared 已恢复基础设施中立，GORM/pgx 适配器、29 个生产调用文件、错误兼容、依赖门禁及无外部依赖工程门禁均通过。GORM CRUD 集成和完整 CI 因缺少隔离 PostgreSQL/Redis 环境保持 blocked，不将其记为通过。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-08-01 | 建立实施前验收模板，等待独立评审。 |
| v0.2 | 2026-08-01 | 按评审补充 GORM/wrapped error 契约及 PostgreSQL/Redis 分层环境门禁。 |
| v0.3 | 2026-08-01 | 记录 shared 边界 RED/GREEN、错误映射、29 个调用点编译及工程门禁结果。 |
| v0.4 | 2026-08-01 | 按实现复审使用 `strconv.Unquote` 解析 import literal，并增加双引号与反引号临时源码回归。 |
| v1.0 | 2026-08-01 | 独立实现复审无新的 P0/P1/P2；以 accepted_with_risk 记录隔离 PostgreSQL/Redis 缺失并归档。 |
