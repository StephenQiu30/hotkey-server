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
status: archived
execution_status: done
version: v2.1
owner: HotKey Server Team
depends_on: [PRD-001]
design_refs:
  - docs/design/archive/002-后端单体架构设计.md
  - docs/design/archive/003-数据库与数据生命周期设计.md
canonical_path: docs/prd/archive/002-单一Schema与数据库平台.md
inputs:
  - docs/design/archive/002-后端单体架构设计.md
  - docs/design/archive/003-数据库与数据生命周期设计.md
outputs:
  - 数据库运行时、事务与兼容性平台需求
triggers:
  - 数据库结构或 Repository 契约变化
downstream:
  - docs/plans/archive/002-单一Schema与数据库平台计划.md
  - docs/acceptance/archive/002-单一Schema与数据库平台验收.md
---

# 数据库运行时、事务与兼容性平台

## 目标

在 PRD-001 已验证的完整 `db/schema.sql`、记录模型和 Repository 契约之上，建立 PostgreSQL、GORM、pgx、事务、分页、错误映射和只读兼容性基础设施。该任务不重新定义数据库事实源。

## 当前差距

- 当前最小骨架尚无受控数据库连接池、GORM、事务管理器、兼容性校验和 Repository 具体实现。
- 完整 Schema、记录模型和接口契约由 PRD-001 原子交付；本任务必须消费并保持该已验证基线，而不能重建或分片它。

## 范围

- 接入 PostgreSQL 16+、pgvector、pgx v5、GORM v2。
- 唯一网络连接池为 `*pgxpool.Pool`；平台仅从该 pool 导出 `database/sql` facade 供 GORM 使用，禁止第二个 DSN、第二个 pool 或独立 GORM 连接。River 运行时不在本任务接入，PLAN-013 只能消费同一平台导出的 pool。
- 在根 `db` 包以 `go:embed schema.sql` 暴露唯一 Schema 内容；提供显式的空库 `db init --empty-only` 和只读 `db verify` 命令；服务启动只做兼容性检查，禁止 AutoMigrate。
- 建立显式事务管理、错误映射、版本化游标分页、乐观锁和通用/受限 Repository 的具体实现。
- 在不修改 Schema 的前提下，为现有记录模型补齐持久化元数据：表名、主键/版本列、删除策略、允许排序及游标键；禁止平行模型。

## 非范围

- 不实现业务用例或公共 CRUD API。
- 不迁移旧生产数据；目标仅支持空库建立。
- 不重新定义表目录、表数量、约束、索引、记录模型或 Schema 基线验收；这些均由 PRD-001 负责。
- 不引入 Goose、`db/migrations` 或任何第二套结构事实源。

## 功能要求

1. 启动只检查版本、扩展和 PRD-001 已建立的关键对象，不静默改表。
2. `pgxpool.Pool` 是唯一连接池；GORM 使用该 pool 派生的 `database/sql` facade。事务管理器以一个 `*sql.Tx` 同时绑定 GORM 与参数化原生 SQL，禁止在同一事务闭包混用独立 pgx transaction。River 仅预留同一 pool 的未来注入点。
3. `db init --empty-only` 必须显式确认目标 DSN、取得 advisory lock、确认无用户表后执行嵌入二进制的唯一 Schema；失败回滚且不留下半初始化结构。`db verify` 必须在只读事务中验证服务器版本、扩展、精确对象集、关键约束与 catalog 指纹，且不写库。
4. Repository 实现将 not found、SQLSTATE 23505/23503/23514、取消/超时、序列化失败和零行乐观锁映射为稳定领域错误。
5. 软删除、状态归档、关系硬删除和不可变运行历史遵守 PRD-001 已定义的模型和契约分类。
6. 事务提交、错误/ panic 回滚、上下文取消后的连接复用、并发乐观锁和版本化游标分页在真实 PostgreSQL 集成测试中可复核；游标必须含版本、排序/筛选指纹和 `id` tie-breaker，并拒绝无效、过期或越权排序。

## 交付物

- database 平台包、事务管理器、分页与 Repository 具体实现。
- `db init --empty-only`、`db verify` 与启动兼容性检查命令。
- 真实 PostgreSQL 的连接池、事务、CRUD、错误映射、并发更新和分页测试。
- 防止 AutoMigrate、Migration、分片 Schema 与平行模型回流的运行时门禁。

## 验收标准

- `db init --empty-only` 只在空库成功；`db verify` 对兼容库成功、对缺扩展/关键对象/版本不符的库安全失败且不改表。
- 应用启动不执行 AutoMigrate；PLAN-002 验证 pgxpool 与 GORM 的受控连接和事务边界，PLAN-013 再验证 River 消费同一平台 pool。
- Repository 实现满足 PRD-001 的通用/受限契约；真实 PostgreSQL 覆盖 CRUD、错误映射、提交、回滚、取消和并发乐观锁。
- 核心游标分页与索引在可代表目标容量的数据集上有可复核执行计划。

## 验收数据分层

- 快速集成集使用每场景独立、可丢弃 PostgreSQL+pgvector 库；缺少 `HOTKEY_TEST_DSN` 必须失败，不得跳过。
- 容量集单独生成可复核的内容/监控/事件样本、记录数据量和查询，断言稳定游标计划使用预期索引且不存在无界 `OFFSET`；900 万内容的完整容量证明在 PLAN-017 运行治理验收中保存，PLAN-002 至少提供可重复缩放的生成器和解释计划门禁。

## 完成定义

本任务完成后，所有后续 PRD 只能在变更经设计和计划批准后增量维护 PRD-001 创建的完整 `db/schema.sql`，不得创建第二套数据库事实源。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-15 | 建立单一 Schema 与数据库平台范围 |
| v2.0 | 2026-07-16 | 将 Schema、记录模型和 Repository 契约基线归入 PRD-001；本任务聚焦运行时、事务、兼容性和具体实现 |
| v2.1 | 2026-07-16 | 固化单一连接池、事务绑定、持久化元数据、命令安全与分层数据库验收契约 |
