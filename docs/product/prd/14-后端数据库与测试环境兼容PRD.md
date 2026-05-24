---
layer: PRD
doc_no: "14"
audience:
  - PM
  - Dev
  - QA
feature_area: backend-runtime
purpose: "定义后端在本地与 CI 中数据库策略边界，确保 PostgreSQL 的可复测性。"
canonical_path: "docs/product/prd/14-后端数据库与测试环境兼容PRD.md"
status: draft
version: "0.1.0"
owner: "StephenQiu30"
inputs:
  - "server/app/core/settings.py"
  - "server/app/db/init_schema.py"
  - "server/app/db/session.py"
outputs:
  - "数据库连接策略与初始化流程可复现"
  - "PostgreSQL 下模型建表与 JSON 行为一致"
triggers:
  - "pytest/CI 在缺少可用数据库时可验证连接与初始化边界"
  - "本地/测试与生产数据库策略不一致导致回归"
downstream:
  - "docs/plans/25-后端数据库兼容与测试可复测计划.md"
  - "server/app/db/"
  - "tests/test_db_connection.py"
---

# 后端数据库与测试环境兼容 PRD（D0）

## 1. 背景

后端核心链路已具备 P0 能力闭环，但数据库应明确约束为 PostgreSQL，避免 SQLite 混入引发行为分叉。

## 2. 目标

- 明确数据库 URL 的解析规则：仅允许 PostgreSQL 方言（含 driver 变体）。
- 按 PostgreSQL 方言执行统一、可复现的初始化策略。
- 保证 JSON 字段在 PostgreSQL 下可被业务代码正确访问。
- 用单元测试把连接解析和初始化分支固化，杜绝回归。

## 3. 需求定义

### 3.1 连接策略
- 新增数据库连接解析入口（`server/app/db/connection.py`）。
- 规则：
  - `DATABASE_URL` 仅允许 `postgresql` 与 `postgresql+psycopg` 等 PostgreSQL 方言（含 driver 变体）。
  - 不支持的数据库 URL 直接报错，避免隐式降级到其他数据库。

### 3.2 初始化策略
- `server/app/db/init_schema.py` 使用统一解析后的 URL。
- PostgreSQL 路径继续使用 `sql/001_init_schema.sql` 执行初始化。

### 3.3 JSON 字段可复测性
- `server/app/models` 的 JSON 字段（`Hotspot.raw_payload`、`AiAnalysis.raw_response`、`Setting.value`、`Source.config`）使用 `JSON().with_variant(JSONB, "postgresql")`，并覆盖关键读写回归。

### 3.4 查询兼容
- Hotspot 聚类版本与排序表达式在 PostgreSQL 下保持兼容实现。
- 查询逻辑重点验证 PostgreSQL 分支。

### 3.5 数据库选型调研结论
- 对比维度：数据特性、查询需求、现有运行时能力、运维一致性。
- 决策结论：项目采用 PostgreSQL，**不计划支持 MySQL 或 SQLite 作为正式运行库**，原因如下：
  - 数据模型与初始化 SQL 大量依赖 PostgreSQL 特性：`JSONB`、`TIMESTAMPTZ`、分区/部分索引语义（`WHERE` 条件下的唯一索引）、以及 `BIGSERIAL`。
  - 业务层存在高频 JSON 写读与排序聚合，PG 的 JSONB 表达能力与索引策略更贴近现有数据形态（`sql/001_init_schema.sql` 已显式声明 `JSONB` 和 `jsonb` 字段默认值）。
  - 项目依赖与部署默认链路已经稳定约定 `postgres` 服务（`docker-compose*.yml`、`.env.example`、`server/app/core/settings.py`），保持单一数据库减少环境分叉与运维成本。
- 约束要求：本 PRD 只固定 PostgreSQL 的连接与初始化链路，避免运行时静默切换，所有兼容性验收以 PostgreSQL 为准。

## 4. 非目标

- 不新增数据库供应商支持。
- 不引入迁移工具（当前仍以可复现初始化为主）。
- 不改变现有业务模型含义与接口。

## 5. 验收标准

### 5.1 自动化验收
- `python3 -m pytest -q tests/test_db_connection.py` 全部通过。
- `python3 -m pytest -q` 全量通过。

### 5.2 行为验收
- PostgreSQL URL 下初始化执行 `sql/001_init_schema.sql`。
- 非支持数据库 URL 报错，且不出现隐式降级逻辑。

## 6. 风险与边界

- `sql/001_init_schema.sql` 为 PostgreSQL 方言语义权威源；后续变更与环境验证统一在 PostgreSQL 下执行。
- 生产运行时数据库特性差异需在对应数据库环境下再次回归验证。

## 7. 下游任务

- `docs/plans/25-后端数据库兼容与测试可复测计划.md`
- `docs/plans/22-后端P0任务编排总控表.md`（加入 D0 任务行）
- `docs/plans/23-企业级P0任务一次性拆分与排程.md`（保留企业级与非企业级边界）

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 0.1.0 | 修订 D0 PRD：取消 SQLite 回退，并确定 PostgreSQL 单一数据库策略 |
