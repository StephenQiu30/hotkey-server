---
layer: PRD
prd_no: "013"
doc_no: "013"
title: Cron与River主链路编排
audience: [Dev, QA, Ops]
feature_area: 可靠任务编排
purpose: 定义 Cron 调度与 River P0 主链路编排任务
phase: P0
priority: P0
status: review
execution_status: backlog
version: v1.0
owner: HotKey Server Team
depends_on: [PRD-006, PRD-007, PRD-008, PRD-009, PRD-010, PRD-011, PRD-012]
design_refs:
  - docs/design/archive/002-后端单体架构设计.md
  - docs/design/archive/005-数据来源查询规划与采集设计.md
  - docs/design/012-监控调度与River流水线设计.md
canonical_path: docs/prd/013-Cron与River主链路编排.md
inputs:
  - docs/design/archive/002-后端单体架构设计.md
  - docs/design/archive/005-数据来源查询规划与采集设计.md
  - docs/design/012-监控调度与River流水线设计.md
outputs:
  - P0 Cron 与 River 编排需求
triggers:
  - Job、幂等、检查点、重试或调度规则变化
downstream:
  - docs/plans/013-Cron与River主链路编排计划.md
  - docs/acceptance/013-Cron与River主链路编排验收.md
---

# Cron 与 River 主链路编排

## 目标

把已实现的 P0 能力接入持久化 Cron/River 流水线，满足事务入队、幂等、检查点、重试、背压、取消和进程恢复。

## 范围

- 接入 pgx v5 与 River，完成 api/worker/all 角色装配。
- Cron 只扫描到期对象并提交唯一任务，不直接执行采集或业务计算。
- 实现 collect_source、normalize_content、evaluate_relevance、cluster_content、recompute_event_heat、generate_event_summary Job。
- 为每个 Job 定义版本化载荷、稳定幂等键、超时、队列、优先级和重试分类。
- 实现事务入队、运行状态、检查点推进、隔离、取消、回放和重算。
- 提供队列与流水线运行查询、重试和取消管理 API。

## 非范围

- 不引入 Kafka、内部事件总线或通用工作流引擎。
- 不把大段正文或密钥放入 Job 载荷。
- 不在 Cron 进程内维护业务事实或检查点。

## 功能要求

1. 业务状态写入与下游 Job 提交处于同一数据库事务。
2. Job 载荷只包含稳定 ID、版本、时间窗、哈希和关联 ID。
3. 同一幂等键只存在一个有效执行，重复任务安全退出。
4. source checkpoint 仅在运行项与下游入库对账完成后推进。
5. 429/临时错误重试，认证/Schema/永久错误隔离，不无限重试。
6. Worker 使用 context 取消、有界并发和队列背压。
7. 暂停 Monitor 不提交新采集，但共享全局 Event 不被删除。
8. 进程在任意 Job 前后退出，恢复后不重复业务事实。

## 交付物

- River 客户端、Worker、Cron 调度器和六类 P0 Job。
- 事务入队、幂等策略、运行查询和管理 API。
- Schema/River 初始化、记录模型、OpenAPI、指标与告警基线。
- 崩溃、重启、重复、取消、背压、单来源故障和回放测试。

## 验收标准

- RSS/HN → Content → MonitorMatch → Event → Heat/Trend → evidence-backed summary → Event API 本地链路通过。
- 95% 正常可采集内容在 60 分钟内形成或更新 Event。
- 同一时间片重跑不重复 Content、匹配、Event、证据或 AI 运行。
- 单来源、LLM、Vault 或 SMTP 故障不破坏 P0 结构化热点查询。
- api 与 worker 分进程运行时不依赖进程内共享状态。

## 完成定义

P0 热点事件监控主链路可持续运行；P1 任务只需增加独立 Job，不改写 P0 编排契约。
