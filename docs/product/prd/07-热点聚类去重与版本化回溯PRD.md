---
layer: PRD
doc_no: "07"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
  - Ops
feature_area: backend-data
purpose: "定义热点聚类、去重和版本化元数据可检索回溯能力（A4）在后端可交付边界。"
canonical_path: "docs/product/prd/07-热点聚类去重与版本化回溯PRD.md"
status: draft
version: "0.1.0"
owner: "StephenQiu30"
inputs:
  - "docs/engineering/验收标准.md（A4）"
  - "server/app/services/check_runner.py"
  - "server/app/api/routes/hotspots.py"
outputs:
  - "聚类回溯 API 与版本元数据查询"
  - "版本化检索与运营可读性说明"
triggers:
  - "触发企业级 P0 去重与聚类补齐"
  - "热点重复、串联事件需跨批次追踪"
downstream:
  - "docs/plans/14-热点聚类与回溯查询计划.md"
  - "docs/engineering/验收标准.md"
---

# 热点聚类、去重与版本化回溯 PRD（A4）

## 1. 背景

当前热点入库已用 `source_id + url` 去重并保留 `cluster_id`，但缺少“可按聚类回溯历史版本和来源变体”的 API。企业级 P0 需要对事件族群做版本化追踪，以便支持去重验证与运营复盘。

## 2. 目标（SMART）

- 保持现有 `check_runner` 去重行为不变。
- 新增聚类版本元数据字段（若已有则补齐）与查询能力，能够按 `cluster_id` 检索同事件下所有记录。
- 提供按热点 `id` 的聚类溯源接口，用于回溯过去 N 次抓取中的同类记录。
- 不影响现有热点列表和详情接口兼容性。

### 验收标准

- 能返回指定 `cluster_id` 下的热点集合，按抓取时间倒序。
- 能返回任一热点的 `cluster_version`（同聚类内排序号）与变更时间线。
- 可针对不存在聚类返回空列表，不抛异常。
- 对回溯查询添加最少两条单测。

## 3. 非目标

- 不实现语义向量聚类，不引入外部向量数据库。
- 不新增实时事件流系统。
- 不改造前端展示页（仅提供接口与后端数据可查询性）。

## 4. 功能定义

- 数据标准：
  - `cluster_id` 来源于标准化标题/规则生成；
  - `cluster_version` 标识同 `cluster_id` 下同一来源或历史顺序；
  - 版本元数据可由 `raw_payload` 表达，不新增独立表。
- API 形态：
  - `GET /api/hotspots/cluster/{cluster_id}`：返回聚类内热点列表；
  - `GET /api/hotspots/{hotspot_id}/cluster-history`：返回同聚类时间线（最多 `limit` 条）。
- 兼容约束：
  - 聚类字段缺失时，返回 404/空列表中的一种可控行为，不因数据脏值失败。

## 5. 风险与边界

- 查询基于 JSONB 字段，PostgreSQL 上有可用性；如需性能优化可后续扩展 GIN/表达式索引。
- `cluster_version` 可能受历史数据缺失影响，回溯时需容错缺值场景。

## 6. 交付与验收

- 在 `check_runner` 中补齐聚类版本元数据写入逻辑。
- 在 `server/app/api/routes/hotspots.py` 增加聚类回溯与时间线接口。
- 通过 `tests/test_mvp_services.py` 增加聚类回溯与版本化查询测试。
- 更新 `docs/验收标准.md` 对 A4 做回填状态管理。

## 7. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 0.1.0 | 新建企业级 A4 补齐 PRD |
