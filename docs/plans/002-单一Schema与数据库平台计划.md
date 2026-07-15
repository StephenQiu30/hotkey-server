---
layer: Plan
doc_no: "002"
audience: [Dev, QA, Ops]
feature_area: 数据库运行时
purpose: 在既有Schema基线上建立数据库运行时、事务、兼容性和Repository实现
canonical_path: docs/plans/002-单一Schema与数据库平台计划.md
status: accepted
execution_status: backlog
review_status: pending
version: v2.0
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

- 当前 Plan 的 status 为 accepted、review_status 为 approved、execution_status 为 ready
- PLAN-001 execution_status 为 done
- 本地 PostgreSQL 16+ 与 pgvector 可创建可丢弃空测试库
- PRD-002 与设计 003 已 accepted

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 创建 | internal/platform/database/pool.go | pgx 连接池 |
| 创建 | internal/platform/database/gorm.go | GORM 装配 |
| 创建 | internal/platform/database/schema.go | 初始化与兼容性检查 |
| 创建 | internal/platform/database/transaction.go | 显式事务边界 |
| 创建 | cmd/hotkey/db.go | db init 与 db verify 命令 |
| 创建 | internal/shared/repository/errors.go | 数据库错误映射 |
| 创建 | internal/shared/repository/gorm.go | 通用与受限 Repository 的 GORM 实现 |
| 创建 | internal/shared/pagination/cursor.go | 游标分页 |
| 创建 | internal/platform/database/*_test.go | 真实 PostgreSQL 的连接、事务、兼容性与取消测试 |
| 创建 | internal/shared/repository/*_integration_test.go | CRUD、错误映射、乐观锁和受限历史测试 |
| 修改 | cmd/hotkey/main.go、go.mod、go.sum、Makefile、AGENTS.md | 运行时装配、依赖和验证入口 |

## 执行步骤

1. 先写连接失败、缺扩展/关键对象、事务回滚、取消、并发乐观锁、分页和错误映射红灯测试；Schema、模型和关键约束应直接复用 PLAN-001 的已通过门禁。
2. 接入 pgx、GORM 和受控连接池，禁止 AutoMigrate；让应用启动只读检查版本、扩展与关键对象。
3. 实现显式事务管理及其在提交、回滚、panic/错误、取消下的资源释放。
4. 实现基于 PLAN-001 接口与记录模型的通用/受限 Repository、数据库错误映射、乐观锁和游标分页。
5. 实现 `db init --empty-only`（显式执行既有 Schema）和只读 `db verify`，对非空库、缺对象和不兼容扩展安全失败。
6. 在可丢弃 PostgreSQL 测试库运行真实集成测试、查询计划与容量样本；确认本任务不修改 `db/schema.sql`，除非新的设计批准了必要前向修订。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | go test ./internal/platform/database ./internal/shared/repository -count=1 | 因运行时、事务、兼容性或具体 Repository 实现缺失失败 |
| 基线 | go test ./tests/architecture -run 'TestSchema|TestModelSchema' -count=1 | PLAN-001 的 Schema 和模型门禁保持通过 |
| 命令 | go run ./cmd/hotkey db verify | 兼容 Schema 返回 0，命令不改表 |
| 集成 | go test ./internal/platform/database ./internal/shared/repository -count=1 | 真实 PostgreSQL 的事务、取消、CRUD、错误映射和并发测试通过 |
| 绿灯 | go test ./internal/platform/database ./internal/shared/... ./tests/architecture -count=1 | 全部通过 |
| 全量 | make lint && make test && make build && make validate | 全部通过 |

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
