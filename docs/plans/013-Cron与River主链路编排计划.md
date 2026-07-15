---
layer: Plan
doc_no: "013"
audience: [Dev, QA, Ops]
feature_area: 可靠任务编排
purpose: 用 Cron 与 River 编排 P0 热点事件主链路
canonical_path: docs/plans/013-Cron与River主链路编排计划.md
status: review
execution_status: backlog
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/013-Cron与River主链路编排.md
  - docs/plans/006-查询规划与RSS-HN采集计划.md
  - docs/plans/007-内容标准化去重与MinIO证据计划.md
  - docs/plans/008-AIProvider与Embedding基础计划.md
  - docs/plans/009-多语言相关性匹配与反馈计划.md
  - docs/plans/010-事件聚类生命周期与人工治理计划.md
  - docs/plans/011-热度趋势与监控排序计划.md
  - docs/plans/012-证据化事件摘要实体与主张计划.md
outputs:
  - Cron 调度与六类 P0 River Job
  - 可恢复 P0 主链路
triggers:
  - PRD-013 accepted 且 ready
downstream:
  - docs/acceptance/013-Cron与River主链路编排验收.md
depends_on: [PLAN-006, PLAN-007, PLAN-008, PLAN-009, PLAN-010, PLAN-011, PLAN-012]
---

# Cron 与 River 主链路编排计划

## 计划目标

把 006–012 的同步能力接入持久化 Job，使重复、崩溃、取消和单来源故障不会破坏业务事实。

## 开工条件

- 对应 PRD 的 status 为 accepted，execution_status 为 ready
- frontmatter 中 depends_on 列出的 Plan 全部为 done
- main 已同步，工作区只包含当前任务相关文件

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 创建 | internal/platform/queue/river.go | River 客户端与 Worker |
| 创建 | internal/platform/scheduler/cron.go | 到期扫描与唯一入队 |
| 创建 | internal/bootstrap/worker.go | worker/all 装配 |
| 创建 | internal/modules/source/infrastructure/jobs/collect.go | collect_source |
| 创建 | internal/modules/ingestion/infrastructure/jobs/*.go | normalize 与 relevance |
| 创建 | internal/modules/event/infrastructure/jobs/*.go | cluster 与 heat |
| 创建 | internal/modules/intelligence/infrastructure/jobs/summary.go | summary |
| 创建 | internal/modules/operations/transport/http/jobs.go | 运行、重试和取消 API |
| 修改 | db/schema.sql | 运行状态与 River 基础设施 |
| 修改 | internal/platform/config/config.go | 队列、并发、超时和 Cron |
| 创建 | tests/integration/pipeline_test.go | P0 端到端与恢复测试 |

## 执行步骤

1. 先写幂等键、事务入队、检查点和崩溃恢复红灯测试。
2. 接入 River 与 worker 生命周期。
3. 实现 Cron 到期扫描，只提交唯一任务。
4. 为六类 Job 定义稳定载荷、重试、超时和取消。
5. 实现检查点推进、隔离、回放和运行查询。
6. 用 RSS/HN Fixture 跑通完整 P0 链路。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | go test -tags=integration ./tests/integration -run Pipeline -count=1 | 因队列与 Job 缺失失败 |
| 绿灯 | go test ./internal/platform/queue ./internal/platform/scheduler ./internal/modules/... -count=1 | Job 单元测试通过 |
| 恢复 | go test -tags=integration ./tests/integration -run PipelineRecovery -count=1 | 重启与重复执行通过 |
| 主链路 | go test -tags=integration ./tests/integration -run RSSHNPipeline -count=1 | P0 链路通过 |
| 全量 | make ci && make smoke | 全部通过 |

## 验收清单

- 业务写入与下游 Job 同事务
- 相同幂等键只存在一个有效执行
- 单来源失败不阻塞其他来源
- 任意 Job 前后退出可恢复
- 暂停 Monitor 不再提交新采集
- 无 LLM、Vault 或 SMTP 时 P0 Event API 可用
- 95% 正常内容在 60 分钟内形成或更新 Event

## 提交边界

- test: 定义 River 幂等与恢复门禁
- impl: 接入 River、Cron 和 Worker
- feat: 编排 P0 热点主链路


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现
