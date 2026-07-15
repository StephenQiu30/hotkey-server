---
layer: Design
doc_no: 012
audience: [Dev, QA, Ops]
feature_area: 监控调度与可靠任务
purpose: 定义Monitor调度、River任务图、幂等、检查点、重试、取消与恢复契约
canonical_path: docs/design/012-监控调度与River流水线设计.md
status: review
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/design/002-后端单体架构设计.md
  - docs/design/005-数据来源查询规划与采集设计.md
  - docs/design/009-事件发现聚类与生命周期设计.md
  - docs/design/010-热度趋势与排序设计.md
  - docs/design/011-AI任务证据与模型运行设计.md
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

Cron 只执行“查询到期对象并提交唯一任务”，不执行采集或业务计算。调度输入为当前 `monitor_sources` 和 `source_checkpoints.next_poll_at`。

调度时间片使用Monitor配置时区解析，存储为UTC。调度器重启后只补运未成功的最近时间片，不无界回放全部历史窗口。

## 3. Monitor 状态与配置发布

Monitor 使用 `draft/active/paused/archived` 状态：

- `draft`：可编辑规则、来源和AI候选词，只允许执行无副作用预览。
- `active`：按发布版本调度采集和匹配。
- `paused`：不提交新采集任务，历史事件继续可读。
- `archived`：不参与调度和新匹配，只允许管理员恢复或保留策略处理。

规则、词项、来源、阈值或频率的修改使用 `version` 乐观锁事务化发布。只有 `approval_status=approved` 的AI扩展词可进入正式查询签名。发布新版本后：

1. 对新规则生成稳定配置哈希。
2. 使旧的Monitor Embedding和未开始时间片失效。
3. 为各启用来源重算查询签名和下次调度时间。
4. 不改写历史匹配、Event和报告；需要时由管理员显式提交重算。

规则预览使用固定的最近Content样本，返回命中、拒绝、解释和估算外部请求量，但不写入正式 `monitor_matches`、Event 或下游Job。

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
| `collect_source` | `monitor_source_id + schedule_window` |
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

`source_checkpoints` 只在以下条件全部满足时推进：

1. 当前页的原始证据已按来源策略保存或明确跳过。
2. 每个 SourceItem 已写入 Content、标记确定性跳过或记录隔离失败。
3. 下游任务已于同事务提交。
4. `collection_run_items` 数量和分类与来源页面一致。

单条永久坏数据可隔离后推进；数据库、对象存储或整页解析失败不推进。

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

- [后端单体架构设计](002-后端单体架构设计.md)
- [数据来源、查询规划与采集设计](005-数据来源查询规划与采集设计.md)
- [事件发现、聚类与生命周期设计](009-事件发现聚类与生命周期设计.md)
- [热度、趋势与排序设计](010-热度趋势与排序设计.md)
- [AI任务、证据与模型运行设计](011-AI任务证据与模型运行设计.md)

## 17. 待确认问题

具体River版本和队列并发数属实施与本地容量标定，不改变本文的事务、幂等、检查点和降级边界。

## 18. 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-15 | 建立P0/P1任务图、载荷、幂等、事务入队、检查点、取消、重算和验收契约 |
