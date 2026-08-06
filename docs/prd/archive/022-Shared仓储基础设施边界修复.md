---
layer: PRD
scope: backend
doc_no: "022"
audience: [Dev, QA, Ops]
feature_area: 工程目录与依赖边界
purpose: 将 GORM/pgx 仓储实现移出 shared，恢复基础设施单向依赖
canonical_path: docs/prd/archive/022-Shared仓储基础设施边界修复.md
status: accepted
execution_status: done
version: v1.2
owner: HotKey Server Team
inputs:
  - AGENTS.md
  - docs/design/archive/002-后端单体架构设计.md
outputs:
  - 基础设施中立的 shared repository 契约
  - platform/database/repository 下的 GORM/pgx 适配器
  - shared 依赖边界架构门禁
triggers:
  - shared 引入数据库驱动、ORM 或 platform 实现
downstream:
  - docs/plans/archive/022-Shared仓储基础设施边界修复计划.md
depends_on: [PRD-002, PRD-021]
---

# Shared 仓储基础设施边界修复

## 1. 背景与目标

`internal/shared/repository` 应只保存跨模块稳定使用的仓储契约与错误分类，但当前 `gorm.go` 和 `MapError` 直接依赖 GORM、pgx 及 `internal/platform/database/model`，形成 shared 反向依赖 platform/具体驱动的问题。

目标是保留 `PageQuery`、`PageResult`、CRUD/History 接口和稳定错误哨兵在 shared，将 GORM CRUD、历史仓储和数据库错误映射迁入 `internal/platform/database/repository`，使依赖方向统一为“platform/模块基础设施 → shared”。

## 2. 范围

- 移动 GORM CRUD/History 实现及其集中测试，不改变表映射、分页、乐观锁、软删除和不可变历史行为。
- 将 GORM/pgx 错误映射放入 platform 数据库仓储包，29 个生产调用文件改用该适配器；稳定错误仍来自 shared，并保持 `errors.Is` 兼容。
- 增加架构门禁：`internal/shared/**/*.go` 不得导入 `internal/platform`、GORM、pgx、Gin、River 或 MinIO SDK。
- 同步 AGENTS 和中英文 README 的目录说明。

## 3. 非目标

- 不改变业务领域接口、HTTP API、OpenAPI、数据库 Schema、SQL、事务语义或 GoLand 启动入口。
- 不新增通用 `pkg`、微服务、依赖注入框架或 Repository 抽象层。
- 不把模块私有 PostgreSQL Repository 合并到 platform。

## 4. 验收条件

1. `internal/shared` 只保留基础设施中立代码，架构门禁能准确拒绝具体驱动和 platform 反向依赖。
2. GORM CRUD/History 与 `MapError` 位于 `internal/platform/database/repository`，shared 中不再包含 GORM/pgx import。
3. `MapError` 对 GORM not-found、PostgreSQL 冲突/约束/取消及 context 取消仍映射到原 shared 错误哨兵。
4. 29 个生产调用文件编译通过，模块层稳定错误判断行为不变。
5. lint、build、架构/仓库验证及 Schema/OpenAPI 零差异通过；数据库集成测试仅在隔离 DSN 可用时执行，否则记录 blocked。

## 5. 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-08-01 | 固定 shared 契约与 platform 数据库适配器边界，等待独立 Plan Review。 |
| v1.1 | 2026-08-01 | Plan 经独立复审通过，进入实施。 |
| v1.2 | 2026-08-01 | 实现通过独立复审并以 79df138 提交，执行状态收敛为 done 后归档。 |
