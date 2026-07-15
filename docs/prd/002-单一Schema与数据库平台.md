---
layer: PRD
prd_no: "002"
doc_no: "002"
title: 单一Schema与数据库平台
audience: [PM, Dev, QA, Ops]
feature_area: 数据库平台
purpose: 定义单一完整 Schema 与数据库基础设施的实施边界
phase: F0
priority: P0
status: review
execution_status: backlog
version: v1.0
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
  - 单一 Schema 与数据库平台需求
triggers:
  - 数据库结构或 Repository 契约变化
downstream:
  - docs/plans/002-单一Schema与数据库平台计划.md
  - docs/acceptance/002-单一Schema与数据库平台验收.md
---

# 单一 Schema 与数据库平台

## 目标

建立 PostgreSQL、GORM、pgx 和事务基础设施，并把当前分散 SQL 收敛为唯一、完整、可重入的 db/schema.sql。

## 当前差距

- 当前结构位于 db/schema/*.sql 和 manifest.txt，与单一 Schema 设计冲突。
- 尚无数据库连接池、GORM、事务管理器、兼容性校验和 Repository 基础契约。
- 目标 31 张业务表、17 张运行表、约束与索引尚未成为可执行事实。

## 范围

- 创建完整 db/schema.sql，按扩展、核心表、关系表、运行表、约束、索引和 River 基础设施排序。
- 接入 PostgreSQL 16+、pgvector、pgx v5、GORM v2。
- 提供空库初始化和只读兼容性检查；应用启动禁止 AutoMigrate。
- 建立显式事务管理、错误映射、游标分页、乐观锁和通用 Repository 契约。
- 为全部业务表建立记录模型或明确的后续模块占位映射，并用架构测试防止 Schema 漂移。
- 删除 db/schema/ 分片和 manifest，不保留第二套手工 Schema。

## 非范围

- 不实现业务用例或公共 CRUD API。
- 不迁移旧生产数据；目标仅支持空库建立。
- 不引入 Goose 或 db/migrations。

## 功能要求

1. db/schema.sql 可在空 PostgreSQL 数据库幂等执行。
2. 启动只检查版本、扩展和关键对象，不静默改表。
3. GORM 与 River 共享受控 pgx 数据源，但事务边界由平台统一管理。
4. Repository 将 not found、unique violation、foreign key 和 optimistic conflict 映射为领域错误。
5. 软删除、状态归档、关系硬删除和不可变运行历史遵守设计分类。
6. halfvec(1024)、分数范围、状态、唯一键和恰好一个目标等约束在数据库层生效。

## 交付物

- db/schema.sql 和数据库初始化、校验命令。
- database 平台包、事务管理器、分页与 Repository 契约。
- Schema、约束、索引、事务、CRUD 和并发更新测试。
- 架构脚本更新，明确禁止 AutoMigrate、migrations 和分片 Schema 回流。

## 验收标准

- 空库连续执行两次 Schema 均成功且对象一致。
- 31 张业务表和 17 张运行表与设计目录一致。
- 非法状态、非法分数、重复幂等键和错误外键被数据库拒绝。
- 每张业务表满足统一 Repository CRUD 契约；运行历史不能被普通 Update/Delete 篡改。
- 核心游标分页与索引在可代表目标容量的数据集上有可复核执行计划。

## 完成定义

本任务完成后，所有后续 PRD 只能增量维护完整 db/schema.sql，不得创建第二套数据库事实源。
