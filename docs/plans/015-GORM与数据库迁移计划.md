---
layer: Plan
doc_no: 015
audience: Dev, QA, Ops
feature_area: GORM与数据库迁移
purpose: 定义新 ORM 主线、migration 主线、核心 repository 迁移顺序与复杂查询约束
canonical_path: docs/plans/015-GORM与数据库迁移计划.md
status: draft
version: v1.0
owner: Codex
inputs:
  - docs/design/006-Go后端工程与启动架构设计.md
  - docs/design/007-核心数据模型与数据库设计.md
outputs:
  - GORM 数据库初始化主线
  - migration 结构
  - repository 迁移顺序
triggers:
  - 应用骨架收敛后
downstream:
  - docs/plans/016-worker与遗留清理计划.md
---

# 背景

当前数据库访问路径没有统一收敛到新主线。若不单独拆计划，ORM 迁移会和 HTTP、worker、启动重构互相打架。

# 目标

1. 建立 GORM 为唯一 ORM 主链路。
2. 建立 migration 为正式 schema 变更主线。
3. 按业务域迁移 repository。
4. 明确复杂查询封装边界，避免 ORM 化后性能和可维护性失控。

# 非目标

1. 本计划不直接完成所有旧路由清理。
2. 本计划不把 `AutoMigrate` 当成生产正式方案。

# Task 1: 建立 database 初始化与 migration 主线

目标：

1. 建立 `internal/platform/database` 初始化路径。
2. 建立 `db/migrations/` 正式结构。
3. 冻结 `db/schema.sql` 在过渡期的历史化角色。

验证门禁：

```bash
go test ./internal/platform/database ./...
```

# Task 2: 迁移核心 CRUD 域 repository

迁移优先级建议：

1. auth
2. monitor
3. notify
4. alert

要求：

1. 使用统一 ORM 模型与 repository interface。
2. service 层不直接依赖 `*gorm.DB`。

验证门禁：

```bash
go test ./tests/unit/auth ./tests/unit/monitor ./tests/unit/notify ./tests/unit/alert ./...
```

# Task 3: 迁移分析型与读模型查询

迁移优先级建议：

1. content
2. topic
3. trend
4. daily digest
5. admin/ops queries

要求：

1. 复杂查询封装在 repository 内部。
2. 聚合查询允许使用 `GORM Raw` 或 builder，但不得泄漏到业务层。
3. 为聚合结果定义专用查询结果模型。

验证门禁：

```bash
go test ./tests/unit/content ./tests/unit/topic ./tests/unit/trend ./tests/integration/... ./...
```

# Task 4: 收敛旧 schema 与旧数据访问路径

目标：

1. 旧 raw SQL 主路径不再作为未来目标架构。
2. 旧生成代码或旧数据库主线降级为待删除遗留。
3. schema 权威切换到 migration 主线。

验证门禁：

```bash
rg -n "database/sql|sqlc" internal cmd
rg -n "db/schema.sql" README.md docs/operations scripts
```

# 风险与边界

1. 如果 CRUD 域和分析域不分开迁移，复杂查询会把 ORM 迁移节奏拖垮。
2. 如果 schema 权威不先切换，ORM 模型会反向驱动数据库漂移。

# 变更记录

## v1.0

1. 新建 GORM 与数据库迁移计划。
2. 将 database init、migration、CRUD 域迁移和分析查询迁移拆成独立阶段。
