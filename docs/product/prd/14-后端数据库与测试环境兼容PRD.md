---
layer: PRD
doc_no: "14"
audience:
  - PM
  - Dev
  - QA
feature_area: backend-runtime
purpose: "定义后端在本地与 CI 中的数据库兼容与可复测性边界，避免环境依赖阻塞开发与回归。"
canonical_path: "docs/product/prd/14-后端数据库与测试环境兼容PRD.md"
status: draft
version: "0.1.0"
owner: "StephenQiu30"
inputs:
  - "apps/api/app/core/settings.py"
  - "apps/api/app/db/init_schema.py"
  - "apps/api/app/db/session.py"
outputs:
  - "数据库连接策略与初始化流程可复现"
  - "SQLite/PostgreSQL 下 JSON 与 JSONB 行为一致"
triggers:
  - "pytest/CI 无法在未配置 PostgreSQL 的环境稳定运行"
  - "本地开发数据库环境与生产数据库一致性检查不足"
downstream:
  - "docs/plans/25-后端数据库兼容与测试可复测计划.md"
  - "apps/api/app/db/"
  - "tests/test_db_connection.py"
---

# 后端数据库与测试环境兼容 PRD（D0）

## 1. 背景

后端核心链路已具备 P0 能力闭环，但开发与 CI 目前仍依赖完整 PostgreSQL 才能稳定执行数据库初始化和运行单元测试。若本地无可用 PostgreSQL，测试会因连接和 JSON 类型差异（JSONB）中断，导致迭代成本上升。

本 PRD 以“开发与交付稳定性”为目标，限定数据库兼容边界：**生产仍走 PostgreSQL 与 `sql/001_init_schema.sql`，测试/本地可通过安全 fallback 保证可复现。**

## 2. 目标

- 明确数据库 URL 在运行时的解析策略，确保 pytest 与开发环境在缺少 PostgreSQL 时可降级到 SQLite。
- 统一初始化行为：PostgreSQL 与测试 SQLite 的 schema 初始化路径不冲突。
- 使 JSON 字段在 SQLite/PostgreSQL 上都可运行，避免启动与查询时的类型不兼容。
- 提供明确的可验证单测，防止回归。

## 3. 需求定义

### 3.1 连接策略
- 新增数据库连接解析入口（`apps/api/app/db/connection.py`）。
- 规则：
  - 非 SQLite 的 `DATABASE_URL` + pytest 运行上下文 -> 自动切到 `sqlite:///` 临时文件。
  - 已明确 SQLite 的 URL -> 原值透传。
  - 非 pytest 场景保持现有 `settings.database_url`。

### 3.2 初始化策略
- `apps/api/app/db/init_schema.py` 使用统一解析后的 URL。
- 当数据库 URL 为 SQLite 时，用 `Base.metadata.create_all()` 初始化模型元数据，不再执行 `sql/001_init_schema.sql`。
- PostgreSQL 路径继续从 `sql/001_init_schema.sql` 初始化，保持事实源一致。

### 3.3 JSON 字段可移植性
- `apps/api/app/models` 的 JSON 字段（`Hotspot.raw_payload`、`AiAnalysis.raw_response`、`Setting.value`、`Source.config`）应使用 `JSON().with_variant(JSONB, "postgresql")`。
- 避免在 JSONB 特定路径上读取时直接 `.astext` 引发兼容性错误。

### 3.4 查询兼容
- Hotspot 聚类版本与排序表达式在 JSON 读写上采用兼容性方式。
- 查询逻辑在 SQLite 回归路径下可通过 `pytest` 通过。

## 4. 非目标

- 不做新数据库供应商支持。
- 不引入复杂迁移/版本管理工具。
- 不改变生产接口形态与现有业务模型字段语义。

## 5. 验收标准

### 5.1 自动化验收
- `python3 -m pytest -q tests/test_db_connection.py` 全部通过。
- `python3 -m pytest -q` 全量通过。

### 5.2 行为验收
- 在非 SQLite 的 `DATABASE_URL` 下，pytest 上下文里解析为 sqlite URL。
- 在 SQLite 路径下执行初始化成功，不依赖 `sql/001_init_schema.sql`。
- `hotspots`、`ai_analysis` 关键列表排序及聚类版本查询在测试路径可执行。

## 6. 风险与边界

- 该能力仅用于降低测试与本地开发门槛，不替代 PostgreSQL 生产行为。
- SQLite 与 PostgreSQL 的并发/事务行为不同，生产异常与容量问题仍以 PostgreSQL 验证为准。

## 7. 下游任务

- `docs/plans/25-后端数据库兼容与测试可复测计划.md`
- `docs/plans/22-后端P0任务编排总控表.md`（加入 D0 任务行）
- `docs/plans/23-企业级P0任务一次性拆分与排程.md`（保留企业级与非企业级边界）

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 0.1.0 | 新增 D0 PRD：后端数据库兼容与测试可复测边界 |
