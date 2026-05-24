---
layer: PRD
doc_no: "24"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
  - Ops
feature_area: hotspot-judgement
purpose: "统一热点事件判定口径，建立可解释热度评分、状态流转与降级规则。"
canonical_path: docs/product/prd/24-热点事件判定与热度引擎PRD.md
status: approved
version: "1.1.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/03-分阶段功能需求（P0/P1）.md
  - docs/engineering/验收标准.md
  - docs/product/prd/00-企业级AI热点监控平台PRD.md
outputs:
  - hotness_score 与 hotness_version 口径
  - status 与阈值流转规则
  - 热点列表/详情字段补齐策略
triggers:
  - "变更相关性/热度阈值模型时"
  - "出现热点误报或误杀争议时"
  - "新增来源特征字段时"
downstream:
  - docs/plans/24-热点事件判定与热度引擎实现计划.md
  - docs/plans/28-里程碑与任务领取总控计划.md
---

# 热点事件判定与热度引擎 PRD（v1）

## 1. 目标（SMART）

1. 在不新增外部 API 的前提下，输出稳定可复现的 `hotness_score`（0-100）与 `hotness_version`。
2. 用统一规则输出 `active` 与 `filtered`，并支持通过配置 `HOTNESS_ACTIVE_THRESHOLD` 调整行为。
3. 在现有主链路不阻断的前提下，补齐热度排序和状态可解释字段。
4. 每个关键规则都能通过 Given/When/Then 场景在测试中被验证。

## 2. 非目标

- 不引入社交关系网络传播模型。
- 不实现跨仓跨区域数据融合。
- 不新增前端页面；仅通过现有 API 字段增强解释性。

## 3. MVP 评分模型

### 3.1 输入字段

- `AiAnalysis.relevance_score`（AI 相关性）
- `Hotspot.published_at / fetched_at`（新鲜度）
- `Source` 配置的可靠权重（来源强度）
- `AiAnalysis.keyword_mentioned`（关键词命中）

### 3.2 输出字段

- `hotness_score`：0~100 数值，保留两位小数。
- `hotness_version`：v1 固定为 `1`。
- `hotness_breakdown`：保存 `ai_relevance/freshness/source_strength/keyword_fit`。
- `hotness_reason`：中文归因摘要。

### 3.3 公式（v1）

`hotness_score = clamp(0.55*ai_relevance + 0.20*freshness + 0.15*source_strength + 0.10*keyword_fit, 0, 100)`

- `ai_relevance`：取 `AiAnalysis.relevance_score`。
- `freshness`：基于发布时间差异映射到 0~100。
- `source_strength`：来源默认 50，支持配置。
- `keyword_fit`：关键词命中为 100，未命中为 60。

### 3.4 配置项

- `HOTNESS_ACTIVE_THRESHOLD`（默认 70）
- `HOTNESS_SOURCE_STRENGTH_DEFAULT`（默认 50）
- `HOTNESS_MIN_FRESHNESS_HOURS`（默认 72）
- `HOTNESS_MAX_SCORE`（默认 100）

## 4. 状态与过滤

- `active`：`hotness_score >= HOTNESS_ACTIVE_THRESHOLD` 且未触发真实性阻断。
- `filtered`：
  - 低热度
  - 重复去重后历史已存在
  - 真实性失败或降级分支

> 低质量事件默认降级而非阻断，保证主链路稳定。

## 5. 与现有接口关系

- 不新增对外接口路径。
- 搜索与列表默认排序改为：`hotness_score` -> `relevance_score` -> `published_at`。
- 历史字段继续兼容，新增字段作为可选返回。

## 6. 验收（Given/When/Then）

1. Given 默认配置，When 高相关且近期事件进入处理，Then `hotness_score >= HOTNESS_ACTIVE_THRESHOLD`。
2. Given 同一来源同一 URL 复采样本，When 查询榜单，Then 只保留单一事件。
3. Given 配置上调 `HOTNESS_ACTIVE_THRESHOLD`，When 复算，Then 事件可由 `active` 变 `filtered`。
4. Given AI 评分缺失，When 打分，Then 记录降级日志并返回可复现默认值。

## 7. 可观测性

- 每次计算记录 `hotspot_id`、`hotness_version`、`hotness_score`、`hotness_breakdown`。
- 在日志中记录 `hotspot_score` span 与阈值分支。

## 8. 风险

- 公式权重偏差导致误报：M1/M2 提供证据与增强流校正。
- 来源波动影响 `source_strength`：必须支持实时回退与重配。

## 9. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 1.1.0 | PRD 再整理，补齐可执行验收与字段口径 |
