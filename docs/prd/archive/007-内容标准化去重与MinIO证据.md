---
layer: PRD
prd_no: "007"
doc_no: "007"
title: 内容标准化去重与MinIO证据
audience: [PM, Dev, QA, Ops]
feature_area: 内容与证据
purpose: 定义内容标准化、去重、MinIO 证据和删除同步任务
phase: P0
priority: P0
status: archived
execution_status: done
version: v1.6
owner: HotKey Server Team
depends_on: [PRD-002, PRD-006]
design_refs:
  - docs/design/archive/003-数据库与数据生命周期设计.md
  - docs/design/archive/006-内容标准化去重与证据设计.md
canonical_path: docs/prd/archive/007-内容标准化去重与MinIO证据.md
inputs:
  - docs/design/archive/003-数据库与数据生命周期设计.md
  - docs/design/archive/006-内容标准化去重与证据设计.md
outputs:
  - Content 与原始证据需求
triggers:
  - Content、去重、对象存储或删除策略变化
downstream:
  - docs/plans/archive/007-内容标准化去重与MinIO证据计划.md
  - docs/acceptance/archive/007-内容标准化去重与MinIO证据验收.md
---

# 内容标准化、去重与 MinIO 证据

## 目标

把各 Connector 的 SourceItem 转换为可追溯 Content，正确处理重复、原始证据、指标快照和来源删除。

## 范围

- 实现 ingestion 模块四层结构。
- 实现作者、Content、资产、指标快照和采集项到内容的关联。
- 只消费 PLAN-006 已持久化的 CapturedItem；禁止为补取正文、原始响应或证据重新请求来源。
- 统一文本、URL、时间、语言、作者、指标和内容状态。
- 实现来源幂等、精确哈希和近似重复三层判定。
- 定义 ObjectStore 端口并实现 MinIO 适配器、确定性对象键、SHA-256 和元数据。
- 支持 active、invalid、duplicate、deleted、expired 生命周期和删除同步。

## 非范围

- 不把同一事件的不同独立报道折叠为重复。
- 不做 Monitor 相关性、事件聚类或热度计算。
- 未经来源许可不保存完整正文、截图或附件。

## 功能要求

1. source_connection_id + external_id 唯一，重试只能幂等更新。
2. canonical URL、content hash 和 near-duplicate 决策保存目标记录、稳定原因码与算法版本；PLAN-007 只使用严格确定性文本规则，near-text 仅限同一来源，跨来源只允许 exact URL/hash，不引入 Embedding。
3. Content 当前指标与 metric snapshot 的缺失互动指标都以 `null` 保持未知；来源显式返回的 `0` 保持为零。PLAN-006 的 CapturedItem v1 旧 `0` 因缺少存在性信息，必须保守作为未知；新 v2 才可保留显式 `0`。
4. 只将来源配置允许、已持久化的捕获正文安全写入并校验哈希后提交数据库引用；失败可对账清理，不重新请求来源。
5. deleted/expired 内容立即停止参与新匹配、事件、热度、摘要和报告。
6. 所有展示字段可回溯原始链接，证据资产不通过公共 API 任意暴露。
7. 单条解析失败记录原因，不中断同批其他内容。

## 交付物

- ingestion 领域模型、标准化、去重、Repository 和 HTTP 查询用例。
- MinIO ObjectStore 适配器与孤儿对象对账。
- Schema、记录模型、OpenAPI 和状态同步任务。
- URL、哈希、近似重复、指标缺失、MinIO 故障和删除级联测试。

## 验收标准

- 相同来源项重跑只保留一条 Content。
- 精确和近似重复均返回目标记录、原因和版本。
- 独立报道 Fixture 被保留为不同 Content。
- 真实 PostgreSQL + MinIO fixture 在超时、重复上传和数据库回滚时不产生不可解释引用；Delete 失败形成的 orphan 能由真实对象存储对账清理。
- deleted 内容不再出现在查询与下游候选中，重复删除保持幂等。

## 完成定义

PRD-008 和 PRD-009 只处理 active、已标准化且证据可定位的 Content。

PLAN-007 只有在 Design-003 v3.2 和 Design-006 v1.4 经独立 Reviewer 接受、本文档和对应 Plan 均 accepted/approved/ready 后才可实施。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-16 | 初始 Content、去重、MinIO 证据与删除同步需求。 |
| v1.1 | 2026-07-16 | 对齐 durable capture 与 P0 边界：禁止重抓、允许正文证据、可空指标、严格确定性近重复及设计接受门禁。 |
| v1.2 | 2026-07-16 | 对齐独立复核：快照保持未知指标、跨来源近似不折叠，并把真实 PostgreSQL + MinIO 补偿/对账纳入验收。 |
| v1.3 | 2026-07-16 | 补齐既有数据升级和可重复 MinIO fixture 的验收要求。 |
| v1.4 | 2026-07-16 | Design-003 v3.2、Design-006 v1.4 与本 PRD 经独立复核接受，允许进入已批准 PLAN-007 的实施队列。 |
| v1.5 | 2026-07-16 | PLAN-007 已启动执行；逐 Task 的实施证据保留在 Workpad、提交与最终 Acceptance，本文档不记录临时流水。 |
| v1.6 | 2026-07-17 | Acceptance-007 与独立最终复核均通过，归档为 archived/done。 |
