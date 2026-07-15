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
status: review
execution_status: backlog
version: v1.0
owner: HotKey Server Team
depends_on: [PRD-002, PRD-006]
design_refs:
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/006-内容标准化去重与证据设计.md
canonical_path: docs/prd/007-内容标准化去重与MinIO证据.md
inputs:
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/006-内容标准化去重与证据设计.md
outputs:
  - Content 与原始证据需求
triggers:
  - Content、去重、对象存储或删除策略变化
downstream:
  - docs/plans/007-内容标准化去重与MinIO证据计划.md
  - docs/acceptance/007-内容标准化去重与MinIO证据验收.md
---

# 内容标准化、去重与 MinIO 证据

## 目标

把各 Connector 的 SourceItem 转换为可追溯 Content，正确处理重复、原始证据、指标快照和来源删除。

## 范围

- 实现 ingestion 模块四层结构。
- 实现作者、Content、资产、指标快照和采集项到内容的关联。
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
2. canonical URL、content hash 和 near-duplicate 决策保存算法版本与目标记录。
3. 缺失互动指标保持未知，不写为零。
4. 原始对象先安全写入并校验哈希，再提交数据库引用；失败可对账清理。
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
- MinIO 在超时、重复上传和数据库回滚时不产生不可解释引用。
- deleted 内容不再出现在查询与下游候选中，重复删除保持幂等。

## 完成定义

PRD-008 和 PRD-009 只处理 active、已标准化且证据可定位的 Content。
