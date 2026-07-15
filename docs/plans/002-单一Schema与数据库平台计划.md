---
layer: Plan
doc_no: "002"
audience: [Dev, QA, Ops]
feature_area: 数据库平台
purpose: 收敛单一 Schema 并建立数据库、事务和 Repository 基础
canonical_path: docs/plans/002-单一Schema与数据库平台计划.md
status: review
execution_status: backlog
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/002-单一Schema与数据库平台.md
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/plans/001-模块化单体启动与工程门禁计划.md
outputs:
  - 完整 db/schema.sql
  - PostgreSQL、GORM、pgx 和事务平台
  - Repository 基础契约
triggers:
  - PRD-002 accepted 且 ready
  - Schema 或 Repository 契约变化
downstream:
  - docs/acceptance/002-单一Schema与数据库平台验收.md
depends_on: [PLAN-001]
---

# 单一 Schema 与数据库平台计划

## 计划目标

用完整 db/schema.sql 替换分片 Schema，并提供空库初始化、启动兼容性检查、事务、分页和 Repository 基础能力。

## 开工条件

- PLAN-001 execution_status 为 done
- 本地 PostgreSQL 16+ 可创建空测试库
- PRD-002 与设计 003 已 accepted

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 创建 | db/schema.sql | 唯一完整数据库事实源 |
| 删除 | db/schema/*.sql、db/schema/manifest.txt | 移除第二套分片事实源 |
| 创建 | internal/platform/database/pool.go | pgx 连接池 |
| 创建 | internal/platform/database/gorm.go | GORM 装配 |
| 创建 | internal/platform/database/schema.go | 初始化与兼容性检查 |
| 创建 | internal/platform/database/transaction.go | 显式事务边界 |
| 创建 | internal/shared/repository/crud.go | 统一 CRUD 契约 |
| 创建 | internal/shared/repository/errors.go | 数据库错误映射 |
| 创建 | internal/shared/pagination/cursor.go | 游标分页 |
| 修改 | tests/architecture/schema_test.go | 单一 Schema 和禁用 AutoMigrate |
| 创建 | internal/platform/database/*_test.go | 空库、事务、并发与约束测试 |
| 修改 | go.mod、go.sum、Makefile、AGENTS.md | 依赖和验证入口 |

## 执行步骤

1. 先把架构测试改为要求 db/schema.sql，并禁止 db/schema 与 db/migrations。
2. 编写空库、重复执行、约束和事务红灯测试。
3. 按设计顺序合并完整 Schema，删除分片文件。
4. 接入 pgx、GORM、事务和启动兼容性检查，禁止 AutoMigrate。
5. 建立 CRUD、乐观锁、错误映射和游标分页契约。
6. 在本地 PostgreSQL 执行空库、重复初始化和关键约束验证。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | go test ./tests/architecture ./internal/platform/database -count=1 | 因单一 Schema 或数据库平台缺失失败 |
| Schema | psql "$HOTKEY_TEST_DSN" -v ON_ERROR_STOP=1 -f db/schema.sql | 空库执行成功 |
| 幂等 | psql "$HOTKEY_TEST_DSN" -v ON_ERROR_STOP=1 -f db/schema.sql | 第二次执行成功 |
| 绿灯 | go test ./internal/platform/database ./internal/shared/... ./tests/architecture -count=1 | 全部通过 |
| 全量 | make lint && make test && make build && make validate | 全部通过 |

## 验收清单

- 31 张业务表与 17 张运行表进入完整 Schema
- 关键状态、分数、唯一键、外键和 halfvec(1024) 约束生效
- 应用启动只检查兼容性，不修改结构
- Repository 正确映射 not found、conflict 和约束错误
- 分片 Schema、Migration 和 AutoMigrate 无残留

## 提交边界

- test: 定义单一 Schema 与数据库约束
- impl: 建立数据库平台和完整 Schema
- refactor: 删除分片 Schema 并收敛验证入口


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现
