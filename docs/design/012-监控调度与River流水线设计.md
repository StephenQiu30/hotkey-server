---
layer: Design
doc_no: "012"
audience: [Dev, QA, Ops]
feature_area: 监控调度与可靠任务
purpose: 定义Monitor调度、River任务图、幂等、检查点、重试、取消与恢复契约
canonical_path: docs/design/012-监控调度与River流水线设计.md
status: accepted
version: v1.5
owner: HotKey Server Team
inputs:
  - docs/design/archive/002-后端单体架构设计.md
  - docs/design/archive/005-数据来源查询规划与采集设计.md
  - docs/design/009-事件发现聚类与生命周期设计.md
  - docs/design/010-热度趋势与排序设计.md
  - docs/design/archive/011-AI任务证据与模型运行设计.md
  - docs/design/archive/014-监控配置发布与预览设计.md
outputs:
  - P0热点主链路和P1交付链路任务图
  - 任务载荷、幂等键、状态和重试分类
  - 检查点推进、取消、重算、可观测和验收契约
triggers:
  - 新增任务类型或调整调度、依赖、并发、重试与取消规则
  - 修改Monitor启停、检查点或运行观测契约
downstream:
  - HotKey Server实施计划
  - OpenAPI与端到端验收
  - db/schema.sql
---

# 监控调度与 River 流水线设计

## 1. 目标

使用 PostgreSQL 与 River 把“监控词 -> 候选内容 -> 热点事件 -> AI摘要 -> 交付”拆成可追踪、可重试、可取消和可恢复的小任务。任务队列不作为业务事实源，所有进度和结果由项目数据表保存。

## 2. 调度模型

Cron 只执行“查询到期对象并提交唯一任务”，不执行采集或业务计算。调度输入为 active Monitor 的 immutable published config、其中 enabled 的 `monitor_sources`、enabled/non-deleted SourceConnection 与 `source_checkpoints.next_poll_at`；它不得读取 draft。

调度时间片使用Monitor配置时区解析，存储为UTC。调度器重启后只补运未成功的最近时间片，不无界回放全部历史窗口。

## 3. Monitor 状态与配置发布

Monitor 的 `draft/active/paused/archived` 状态、版本化发布、权限、来源引用和纯配置预览以 [Design-014](archive/014-监控配置发布与预览设计.md) 为权威契约。本设计只消费其 published 结果：active Monitor 按当前 published version 调度，paused/archived 不提交新任务，draft 绝不进入调度。

计划 006 必须把 `collection_runs` 作为共享的 `source_connection_id + query_signature + window` 执行事实，并以 `collection_run_targets` 关联 immutable `monitor_source_id + monitor_config_version_id`。每个新 published source 以该 version 的 `published_at` 建空 checkpoint，不继承旧 revision cursor；共享 run 成功且对应 target 成功后才推进该 target checkpoint。发布新版本不改写历史匹配、Event 或报告；需要时由管理员显式提交重算。

P0 配置预览只验证配置、签名和估算，不读取 Content 或写入业务/Job 事实。基于最近 Content 的命中、拒绝和解释预览属于 PRD-009 的相关性匹配能力。

## 4. P0 热点主链路

```text
collect_source
  -> normalize_content
  -> evaluate_relevance
  -> cluster_content
  -> recompute_event_heat
  -> generate_event_summary
  -> project_event_query_model
```

P0 以 RSS/Atom 和 Hacker News 的契约测试及一条真实本地链路验收。`generate_event_summary` 可降级，其失败不得阻塞事件和热度查询。

## 5. P1 知识与交付链路

```text
project_event_to_vault
  -> build_report
  -> validate_report
  -> publish_report
  -> deliver_email
  -> refresh_feed
```

知识和交付任务只消费已提交的 Event 和报告事实。MinIO、Vault、SMTP 或 Feed 失败不回滚已成功的采集和事件事实。

## 6. 任务载荷

每种 Job 使用静态、版本化载荷：

```text
job_version
resource_type, resource_id
idempotency_key
causation_id, correlation_id
requested_at
algorithm_or_contract_version
```

载荷只传递稳定ID和小型标量，不传递完整Content、Prompt、原始响应、密钥或大JSON。Worker 在执行时从业务事实源重读输入并验证版本。

## 7. 幂等键

| Job | 幂等键 |
|---|---|
| `collect_source` | `source_connection_id + query_signature + window_start + window_end` |
| `normalize_content` | `source_connection_id + external_id + payload_hash` |
| `evaluate_relevance` | `content_id + monitor_id + scoring_version + input_hash` |
| `cluster_content` | `content_id + clustering_version + feature_hash` |
| `recompute_event_heat` | `event_id + window_end + heat_version + evidence_set_hash` |
| `generate_event_summary` | AI设计中的 `reuse_key` |
| `project_event_to_vault` | `event_id + event_version + template_version` |
| `build_report` | `report_type + scope + period + version` |
| `deliver_email` | `report_id + subscription_id` |

River 唯一任务只是并发保护；最终幂等由业务表唯一约束和应用层状态转移保证。

## 8. 事务入队

业务事实变更和下游 River Job 必须使用同一 PostgreSQL 事务提交。如适配器无法保证同事务，使用项目 Outbox 表和幂等派发器，不允许“先提交数据、再尽力入队”。V1 优先使用 River 的事务入队能力，不为未使用的队列抽象通用 Outbox 框架。

## 9. 检查点推进

共享 `collection_run` 只执行一次来源请求；每个 `collection_run_target` 的 fetch `source_checkpoint` 只在以下条件全部满足时推进：

1. 当前 shared run 页已完整解析，且原始响应按来源策略持久化、脱敏摘要化或明确跳过。
2. 该 target 的每个 SourceItem 已写入或幂等复用带 captured_item 的 `collection_run_items`，并在 `collection_run_target_items` 标记已捕获、确定性跳过或隔离失败。
3. `collection_run_target_items` 的数量和分类与来源页面一致，且该 target 的捕获结果完整。
4. PLAN-007 的 Content/证据化处理以这些 durable item 另行提交并幂等恢复，不得重新调用 Connector 或回退 fetch checkpoint。

单条永久坏数据可隔离后推进；数据库、对象存储或整页解析失败不推进。某一个 target 的失败不得回滚 shared run 或已成功 target 的 checkpoint，但失败 target 必须保留可重试事实。

## 10. 错误和重试

| 分类 | 示例 | 行为 |
|---|---|---|
| 可重试 | 超时、429、临时5xx、连接中断 | 指数退避、抖动和最大延迟 |
| 不可重试 | 配置错误、授权失效、非法载荷 | 立即失败并提示管理员 |
| 数据隔离 | 单条解析或校验失败 | 记录脱敏原因，批次继续 |
| 人工处理 | Vault冲突、事件拆分、AI证据冲突 | 进入明确复核状态，不循环重试 |

每种 Job 单独设置超时、最大尝试、最大延迟和队列优先级。

## 11. 并发与背压

- 按来源连接限制采集并发和配额。
- 按Provider和任务类型限制AI并发和预算。
- 同一 Event 的成员变更、热度、摘要和Vault写入按稳定键串行化。
- 队列达到阈值时优先保留P0采集、聚类和热度，延后报告和知识提案。
- 不创建无界goroutine，Worker并发由River队列配置和适配器限流共同约束。

## 12. Monitor 启停与取消

- 停用 Monitor 后不再提交新 `collect_source`。
- 尚未开始的Monitor专属采集任务取消。
- 已取得的共享来源页可完成内容入库，但不为已停用Monitor创建新匹配。
- 全局 Event 和历史证据不因停用单个Monitor被删除。
- Worker在外部调用、分页和批处理边界检查 `context` 取消。

## 13. 重算与回放

管理员可按 Content、Event、Monitor、算法版本和时间窗口创建受限重算任务。默认只重算派生事实，不重新调用外部来源。回放使用原始证据、历史版本和新幂等命名空间，不改写已发布报告快照。

## 14. 可观测与运行查询

每个 Job 传播 `request_id`/`trace_id`/`correlation_id`。运行查询至少提供队列深度、最旧等待时间、执行时间、成功率、重试率、隔离数、永久失败数和检查点滞后。日志不包含密钥、完整正文或原始Provider响应。

## 15. 端到端验收

P0 验收固定为：

```text
RSS/HN -> Content -> MonitorMatch -> Event
-> Heat/Trend -> evidence-backed summary -> Event API
```

门禁：

- 95% 的正常可采集内容在 60 分钟内形成或更新 Event。
- 重跑同一时间片不产生重复Content、匹配、Event、证据、AI运行或投递。
- 单一来源失败不阻塞其他来源。
- 进程在任意 Job 前后退出后可通过业务状态和River租约恢复。
- LLM、Vault或SMTP不可用时，P0结构化热点查询仍可用。
- 停用Monitor不再产生新匹配，且不破坏全局Event。

## 16. 关联文档

- [后端单体架构设计](archive/002-后端单体架构设计.md)
- [数据来源、查询规划与采集设计](archive/005-数据来源查询规划与采集设计.md)
- [事件发现、聚类与生命周期设计](009-事件发现聚类与生命周期设计.md)
- [热度、趋势与排序设计](010-热度趋势与排序设计.md)
- [AI任务、证据与模型运行设计](archive/011-AI任务证据与模型运行设计.md)
- [监控配置发布与预览设计](archive/014-监控配置发布与预览设计.md)

## 17. 待确认问题

具体River版本和队列并发数属实施与本地容量标定，不改变本文的事务、幂等、检查点和降级边界。

## 18. 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-15 | 建立P0/P1任务图、载荷、幂等、事务入队、检查点、取消、重算和验收契约 |
| v1.1 | 2026-07-16 | 将 P0 配置状态与预览委托给 Design-014，并同步 immutable published config 与共享 run/target/checkpoint 事实。 |
| v1.2 | 2026-07-16 | 将 `collect_source` River 键和 checkpoint 规则与 shared run/target 模型统一。 |
| v1.3 | 2026-07-16 | 架构审核确认调度、事务入队、幂等、检查点、重试、取消和恢复契约完整；River具体版本与并发数保留为实施容量参数。 |
| v1.4 | 2026-07-16 | 将 fetch checkpoint 固定为 durable collection_run_items 捕获完成，Content/证据化交给 PLAN-007 的可恢复后续处理。 |
| v1.5 | 2026-07-16 | 要求 captured_item 可重放且以 target-item 对账证明每个 immutable target 的捕获完整。 |
