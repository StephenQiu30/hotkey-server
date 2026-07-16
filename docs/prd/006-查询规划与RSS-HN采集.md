---
layer: PRD
prd_no: "006"
doc_no: "006"
title: 查询规划与RSS-HN采集
audience: [PM, Dev, QA, Ops]
feature_area: 来源采集
purpose: 定义查询规划及 RSS、Atom、Hacker News 采集任务
phase: P0
priority: P0
status: archived
execution_status: done
version: v1.9
owner: HotKey Server Team
depends_on: [PRD-005]
design_refs:
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/005-数据来源查询规划与采集设计.md
  - docs/design/012-监控调度与River流水线设计.md
canonical_path: docs/prd/006-查询规划与RSS-HN采集.md
inputs:
  - docs/design/003-数据库与数据生命周期设计.md
  - docs/design/005-数据来源查询规划与采集设计.md
  - docs/design/012-监控调度与River流水线设计.md
outputs:
  - 首批合规来源采集需求
triggers:
  - 查询规划、Connector 或来源策略变化
downstream:
  - docs/plans/006-查询规划与RSS-HN采集计划.md
  - docs/acceptance/006-查询规划与RSS-HN采集验收.md
---

# 查询规划与 RSS/HN 采集

## 目标

实现合规、增量、限流且可恢复的 RSS/Atom 与 Hacker News 第一批数据来源能力。

## 范围

- 定义小型 Connector、SourceItem、分页/游标和来源能力契约。
- 实现查询规划，将已发布 Monitor 词项按来源、语言、地区合并为稳定 query_signature。
- 实现 RSS/Atom 和 Hacker News Connector。
- 实现 source_checkpoints、collection_runs、collection_run_items 的运行记录和状态推进。
- 支持 ETag、Last-Modified、游标、配额、超时、指数退避、熔断和有界并发。
- 实现来源健康查询和管理员安全重试入口。

## 非范围

- 不绕过登录、验证码、反爬或平台限制。
- 不接入需要未获授权抓取的 X、视频或社交页面。
- 不在 Connector 内做内容去重、相关性、事件或 AI 判断。

## 功能要求

1. 同一来源、query_signature 和调度窗口只执行一次外部请求链。
2. Connector 只输出统一 SourceItem，不泄露 SDK 类型。
3. fetch checkpoint 只在页面解析完成、每个 SourceItem 已幂等写入带版本化 `captured_item` 的 `collection_run_items`、每个 immutable target 已在同一 shared run 的 `collection_run_target_items` 对账且运行项计数一致后推进；PLAN-007 以该持久化捕获继续 Content/证据化，不重复请求来源。
4. 401/403、429、5xx、超时和无新内容分别处理。
5. 单一来源失败不影响其他来源完成当前轮次。
6. 大分页可续跑，进程退出后从持久化检查点恢复。
7. 日志、数据库错误和 API 不记录来源密钥或完整原始响应。

## 交付物

- Connector 领域端口、RSS/Atom 和 HN 基础设施适配器。
- 查询规划、检查点、captured item/target-item 对账和采集运行用例。
- Schema、记录模型、管理员运行 API 与指标。
- 契约测试、模拟限流/故障测试和一条可重复本地采集 Fixture。

## 验收标准

- 相同窗口重跑复用同一 shared run，不重复请求、不丢失可重放/脱敏的 captured item 或 target-item 对账、不倒退 fetch checkpoint。
- RSS 条件请求和 HN 游标/ID 增量按契约工作。
- 429 使用 Retry-After 或退避策略，不形成忙循环。
- 单一来源持续失败时被隔离，其他来源仍成功。
- 一轮 P0 来源采集能在小时预算内结束或安全续跑。

## 完成定义

下游只消费 SourceItem 和持久化运行项，不依赖具体 RSS/HN 客户端。

PLAN-006 只有在 Design-003、Design-005 与 Design-012 已接受、本文档已接受且对应 Plan 经独立审核后，才可从 backlog 进入 ready。

## 执行确认

- 2026-07-16：对应 PLAN-006 v1.7 已由独立 Reviewer 审核通过（无 Critical、Important 或 Minor），本文档进入 `accepted/ready`。
- 2026-07-16：Acceptance-006 已 accepted，完整质量门禁和独立最终复核均通过；本文档归档为 `archived/done`。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-16 | 初始查询规划与 RSS/HN 采集需求拆分。 |
| v1.1 | 2026-07-16 | 将任务执行与计划归档治理设为显式前置条件，补充 PLAN-006 的完整开工门禁。 |
| v1.2 | 2026-07-16 | 按产品主链优先级移除治理依赖；保留 Design、PRD、独立审核、测试与验收门禁。 |
| v1.3 | 2026-07-16 | 对齐 Design-005/012：PLAN-006 的完成点是 durable collection_run_items 捕获和 fetch checkpoint，PLAN-007 继续 Content/证据化。 |
| v1.4 | 2026-07-16 | 补齐版本化 captured item、raw disposition 和 target-item 对账，使 PLAN-007 可不重新抓取地恢复 Content/证据化。 |
| v1.5 | 2026-07-16 | 将 Design-003 纳入 Schema 与生命周期的权威输入。 |
| v1.6 | 2026-07-16 | 完成独立审核，接受并切换为 ready。 |
| v1.7 | 2026-07-16 | 开始实施 PLAN-006。 |
| v1.8 | 2026-07-16 | 明确 target-item 对账必须绑定同一个 shared run，拒绝跨 run 捕获事实。 |
| v1.9 | 2026-07-16 | Acceptance-006 与最终独立复核通过，归档为 archived/done。 |
