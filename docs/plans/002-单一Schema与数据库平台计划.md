---
layer: Plan
doc_no: "002"
audience: [Dev, QA, Ops]
feature_area: 数据库运行时
purpose: 在既有Schema基线上建立数据库运行时、事务、兼容性和Repository实现
canonical_path: docs/plans/002-单一Schema与数据库平台计划.md
status: archived
execution_status: done
review_status: approved
version: v2.1
owner: HotKey Server Team
inputs:
  - docs/prd/002-单一Schema与数据库平台.md
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/plans/001-模块化单体启动与工程门禁计划.md
outputs:
  - PostgreSQL、GORM、pgx 和事务平台
  - Repository 具体实现与兼容性命令
triggers:
  - PRD-002 accepted 且 ready
  - Schema 或 Repository 契约变化
downstream:
  - docs/acceptance/002-单一Schema与数据库平台验收.md
depends_on: [PLAN-001]
---

# 数据库运行时、事务与兼容性平台计划

## 计划目标

消费 PLAN-001 已验证的完整 `db/schema.sql`、记录模型和 CRUD 接口契约，提供受控连接、GORM/pgx、事务、分页、错误映射、Repository 具体实现与数据库兼容性命令。不得重新定义或分片 Schema。

## 开工条件

- 当前 Plan 的 status 为 accepted、review_status 为 approved，且已由 `ready` 转为 `in_progress`
- PLAN-001 execution_status 为 done
- 本地 PostgreSQL 16+ 与 pgvector 可创建可丢弃空测试库
- PRD-002 与设计 003 已 accepted

## 审核记录

- 2026-07-16：独立 Reviewer 审核通过。确认唯一 `pgxpool.Pool`、GORM facade、River 延后边界、统一事务 handle、持久化元数据、嵌入 Schema、安全命令和 DSN fixture 验收均已明确。审核结论：`approved`。

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 创建 | internal/platform/database/pool.go | 唯一 pgxpool、database/sql facade 与 Fx 生命周期 |
| 创建 | internal/platform/database/gorm.go | 使用同一 facade 的 GORM 装配 |
| 创建 | internal/platform/database/schema.go | 初始化与兼容性检查 |
| 创建 | internal/platform/database/transaction.go | 显式事务边界 |
| 创建 | db/embed.go | 直接嵌入相邻唯一 `schema.sql`，供平台命令消费 |
| 创建 | internal/bootstrap/db_command.go | db init 与 db verify 命令 |
| 修改 | internal/bootstrap/app.go、internal/platform/config/config.go | 角色装配、启动校验、DSN 与命令解析 |
| 修改 | internal/platform/database/model/model.go | 表/版本/删除/排序/游标持久化元数据 |
| 创建 | internal/shared/repository/errors.go | 数据库错误映射 |
| 创建 | internal/shared/repository/gorm.go | 通用与受限 Repository 的 GORM 实现 |
| 创建 | internal/shared/pagination/cursor.go、internal/shared/pagination/cursor_test.go | 绑定排序方向的游标分页 |
| 创建 | internal/platform/database/*_test.go | 真实 PostgreSQL 的连接、事务、兼容性与取消测试 |
| 创建 | internal/shared/repository/*_integration_test.go | CRUD、错误映射、乐观锁和受限历史测试 |
| 创建 | tests/postgresfixture/fixture.go | 每个集成测试单独创建并销毁数据库 |
| 创建 | scripts/verify-database-runtime.sh、scripts/generate-capacity-fixture.sh | 可丢弃库、DML、catalog 与容量计划门禁 |
| 修改 | internal/shared/repository/crud.go、internal/bootstrap/app_test.go | Repository 契约和运行角色回归覆盖 |
| 修改 | scripts/validate-architecture.sh、scripts/validate-repository.sh | 禁止 AutoMigrate 与业务表元数据门禁 |
| 修改 | go.mod、go.sum、Makefile、AGENTS.md、README.md | 运行时装配、依赖和验证入口 |

## 执行步骤

1. 先写唯一 pool、GORM facade、启动 verify、commit/error/panic/cancel、并发锁、游标和 SQLSTATE 红灯；每场景独立可丢弃 DSN，缺 DSN 失败而不跳过。
2. 创建唯一 `pgxpool.Pool`，从其派生唯一 `*sql.DB` facade 并注入 GORM；Fx 按 pool、facade、GORM 启动并反序关闭。River 不接入，PLAN-013 仅消费该 pool。
3. 事务管理器以 `*sql.Tx` 为唯一 handle，生成绑定同一 handle 的 GORM 会话和参数化原生 SQL executor；基于回调 transaction context 拒绝重入，不隐式使用 savepoint；panic rollback 后 re-panic，取消后归还连接。
4. 补齐每表元数据并实现通用/受限 Repository；Update 必须 `WHERE id + version`，删除遵守 soft/archive/hard/history 策略；游标含版本、排序方向/筛选指纹、`id` tie-breaker、limit 上限。
5. `db init --empty-only` 使用嵌入 Schema、显式确认、advisory lock、空库检查和回滚；`db verify` 在只读事务验证版本、扩展和由嵌入 canonical Schema 派生的明确对象/约束兼容性集合与 catalog 指纹。
6. 执行快速集成与可缩放容量 fixture，保存 EXPLAIN 结果并断言游标使用预期索引、无无界 OFFSET；本任务不改 `db/schema.sql`。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | go test ./internal/platform/database ./internal/shared/repository -count=1 | 因运行时、事务、兼容性或具体 Repository 实现缺失失败 |
| 基线 | go test ./tests/architecture -run 'TestSchema|TestModelSchema' -count=1 | PLAN-001 的 Schema 和模型门禁保持通过 |
| 命令 | `HOTKEY_DATABASE_URL="$HOTKEY_TEST_DSN" go run ./cmd/hotkey db verify` | 兼容 Schema 返回 0，命令不改表 |
| 集成 | `HOTKEY_TEST_DSN="$HOTKEY_TEST_DSN" make database-runtime-verify` | fixture 映射 DSN 并运行真实 PostgreSQL 集成测试 |
| 绿灯 | go test ./internal/platform/database ./internal/shared/... ./tests/architecture -count=1 | 全部通过 |
| 全量 | `HOTKEY_TEST_DSN="$HOTKEY_TEST_DSN" make ci` | 缺 DSN 失败；fixture、lint、测试、构建和门禁全部通过 |

## 验收清单

- 使用并保持 PLAN-001 的完整 Schema、记录模型和关键约束，不创建第二套事实源
- 应用启动只检查兼容性，不修改结构
- Repository 正确映射 not found、conflict 和约束错误
- 事务、取消、乐观锁和游标分页在真实 PostgreSQL 中可复核
- 分片 Schema、Migration、平行模型和 AutoMigrate 无残留

## 提交边界

- test: 定义数据库运行时、事务、兼容性和具体 Repository 红灯
- impl: 建立 pgx/GORM、事务、分页、错误映射与数据库命令
- docs: 记录复用的 Schema 基线与运行时验收证据


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现

不得在本计划重新分片 Schema、修改 Schema 表目录验收或新增平行记录模型。

## 实施变更复核

- 2026-07-16：命令解析位于既有 `internal/bootstrap`，而非薄入口 `cmd/hotkey`；执行文件清单据此修正为 `internal/bootstrap/db_command.go`，目标、范围和验收命令不变。依照 Plan 规范，审核状态已重置为 `pending`，待独立 Reviewer 对实现和该路径修正复核。
- 2026-07-16：独立 Reviewer 复审通过。确认 `db verify` 会比对嵌入 Schema 派生的列、默认值、PK/UQ/FK/CHECK 与显式索引定义；同名约束篡改、索引替换、默认值变更和 public composite type 残留均有回归覆盖。`make ci`、重复集成测试、diff 检查和临时数据库清理均通过。审核结论：`approved`。
