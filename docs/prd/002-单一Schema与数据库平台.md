---
layer: PRD
prd_no: "002"
doc_no: "002"
title: 数据库运行时、事务与兼容性平台
audience: [PM, Dev, QA, Ops]
feature_area: 数据库平台
purpose: 定义既有完整Schema之上的数据库运行时、事务、兼容性与Repository实现边界
phase: F0
priority: P0
status: accepted
execution_status: backlog
version: v2.0
owner: HotKey Server Team
depends_on: [PRD-001]
design_refs:
  - docs/design/002-后端单体架构设计.md
  - docs/design/003-数据库与数据生命周期设计.md
canonical_path: docs/prd/002-单一Schema与数据库平台.md
inputs:
  - docs/design/002-后端单体架构设计.md
  - docs/design/003-数据库与数据生命周期设计.md
outputs:
  - 数据库运行时、事务与兼容性平台需求
triggers:
  - 数据库结构或 Repository 契约变化
downstream:
  - docs/plans/002-单一Schema与数据库平台计划.md
  - docs/acceptance/002-单一Schema与数据库平台验收.md
---

# 数据库运行时、事务与兼容性平台

## 目标

在 PRD-001 已验证的完整 `db/schema.sql`、记录模型和 Repository 契约之上，建立 PostgreSQL、GORM、pgx、事务、分页、错误映射和只读兼容性基础设施。该任务不重新定义数据库事实源。

## 当前差距

- 当前最小骨架尚无受控数据库连接池、GORM、事务管理器、兼容性校验和 Repository 具体实现。
- 完整 Schema、记录模型和接口契约由 PRD-001 原子交付；本任务必须消费并保持该已验证基线，而不能重建或分片它。

## 范围

- 接入 PostgreSQL 16+、pgvector、pgx v5、GORM v2。
- 提供显式的空库 `db init --empty-only` 和只读 `db verify` 命令；服务启动只做兼容性检查，禁止 AutoMigrate。
- 建立显式事务管理、错误映射、游标分页、乐观锁和通用/受限 Repository 的具体实现。
- 使用 PRD-001 的记录模型与 Schema 一致性门禁；运行时不新增平行模型、分片 Schema、Migration 或手工快照。

## 非范围

- 不实现业务用例或公共 CRUD API。
- 不迁移旧生产数据；目标仅支持空库建立。
- 不重新定义表目录、表数量、约束、索引、记录模型或 Schema 基线验收；这些均由 PRD-001 负责。
- 不引入 Goose、`db/migrations` 或任何第二套结构事实源。

## 功能要求

1. 启动只检查版本、扩展和 PRD-001 已建立的关键对象，不静默改表。
2. GORM 与 River 共享受控 pgx 数据源，但事务边界由平台统一管理。
3. `db init --empty-only` 只对显式确认的空测试/本地库执行 PRD-001 的 Schema；`db verify` 只读验证并在不兼容时非零退出。
4. Repository 实现将 not found、unique violation、foreign key 和 optimistic conflict 映射为领域错误。
5. 软删除、状态归档、关系硬删除和不可变运行历史遵守 PRD-001 已定义的模型和契约分类。
6. 事务提交、回滚、上下文取消、并发乐观锁和游标分页在真实 PostgreSQL 集成测试中可复核。

## 交付物

- database 平台包、事务管理器、分页与 Repository 具体实现。
- `db init --empty-only`、`db verify` 与启动兼容性检查命令。
- 真实 PostgreSQL 的连接池、事务、CRUD、错误映射、并发更新和分页测试。
- 防止 AutoMigrate、Migration、分片 Schema 与平行模型回流的运行时门禁。

## 验收标准

- `db init --empty-only` 只在空库成功；`db verify` 对兼容库成功、对缺扩展/关键对象/版本不符的库安全失败且不改表。
- 应用启动不执行 AutoMigrate；连接池、GORM、pgx 与 River 共享受控连接和显式事务边界。
- Repository 实现满足 PRD-001 的通用/受限契约；真实 PostgreSQL 覆盖 CRUD、错误映射、提交、回滚、取消和并发乐观锁。
- 核心游标分页与索引在可代表目标容量的数据集上有可复核执行计划。

## 完成定义

本任务完成后，所有后续 PRD 只能在变更经设计和计划批准后增量维护 PRD-001 创建的完整 `db/schema.sql`，不得创建第二套数据库事实源。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-15 | 建立单一 Schema 与数据库平台范围 |
| v2.0 | 2026-07-16 | 将 Schema、记录模型和 Repository 契约基线归入 PRD-001；本任务聚焦运行时、事务、兼容性和具体实现 |
