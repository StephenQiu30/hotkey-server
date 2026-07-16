---
layer: Plan
doc_no: "006"
audience: [Dev, QA, Ops]
feature_area: 来源采集
purpose: 实施查询规划及 RSS、Atom、Hacker News 采集
canonical_path: docs/plans/006-查询规划与RSS-HN采集计划.md
status: review
execution_status: backlog
review_status: pending
version: v1.1
owner: HotKey Server Team
inputs:
  - docs/prd/006-查询规划与RSS-HN采集.md
  - docs/plans/005-监控主题规则与来源配置计划.md
  - docs/design/014-监控配置发布与预览设计.md
outputs:
  - Connector 契约
  - RSS、Atom、Hacker News 适配器
  - 采集运行与检查点
triggers:
  - PRD-006 accepted 且 ready
downstream:
  - docs/acceptance/006-查询规划与RSS-HN采集验收.md
depends_on: [PLAN-005]
---

# 查询规划与 RSS/HN 采集计划

## 计划目标

交付可增量、限流、可恢复的首批合规来源采集能力，并产生统一 SourceItem。

## 开工条件

- 当前 Plan 的 status 为 accepted、review_status 为 approved、execution_status 为 ready
- 对应 PRD 的 status 为 accepted，execution_status 为 ready
- frontmatter 中 depends_on 列出的 Plan 全部为 done
- main 已同步，工作区只包含当前任务相关文件

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 创建 | internal/modules/source/domain/connector.go | Connector 与 SourceItem |
| 创建 | internal/modules/source/domain/checkpoint.go | immutable MonitorSource 的检查点与 shared-run target 契约 |
| 创建 | internal/modules/source/application/query_planner.go | 稳定查询签名 |
| 创建 | internal/modules/source/application/collection_service.go | 采集运行编排 |
| 创建 | internal/modules/source/infrastructure/rss/*.go | RSS/Atom 适配器 |
| 创建 | internal/modules/source/infrastructure/hackernews/*.go | HN 适配器 |
| 创建 | internal/modules/source/infrastructure/postgres/run_repository.go | shared run、target 与检查点持久化 |
| 创建 | internal/modules/source/transport/http/runs.go | 管理查询与安全重试 |
| 修改 | db/schema.sql | immutable-source checkpoints、shared `collection_runs`、`collection_run_targets` 与 run_items |
| 修改 | internal/bootstrap/app.go、internal/platform/http/router.go | 适配器与路由装配 |
| 创建 | internal/modules/source/**/*_test.go、testdata/sources/* | 契约与 Fixture |

## 执行步骤

1. 先写 Connector 契约、从 published Monitor config 读取的查询签名、条件请求、shared run target 和检查点红灯测试。
2. 实现 SourceItem 与查询规划，不接入下游业务；只消费 Design-014 固定的 query signature、来源配置默认值和 SSRF 约束。
3. 实现 RSS/Atom 的 ETag、Last-Modified 和分页。
4. 实现 HN 增量游标、超时和限流处理。
5. 以 `source_connection_id + query_signature + window` 持久化共享运行，并为每个 immutable MonitorSource/config version 建 target；只有共享运行和目标处理都成功后推进对应 checkpoint。
6. 增加来源隔离、退避、熔断、指标和管理员重试。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | go test ./internal/modules/source/... -count=1 | Connector 与检查点测试失败 |
| 绿灯 | go test ./internal/modules/source/... -count=1 | 全部通过 |
| 契约 | go test ./internal/modules/source/infrastructure/rss ./internal/modules/source/infrastructure/hackernews -count=1 | Fixture 契约通过 |
| 集成 | go test -tags=integration ./internal/modules/source/... | 运行与检查点通过 |
| 全量 | make ci | 全部通过 |

## 验收清单

- 相同来源、查询签名和窗口只执行一次请求链
- 一个共享 run 可服务多个 immutable config target；任一 target 失败不推进其 checkpoint
- 重跑不跳过内容且不倒退检查点
- 401/403、429、5xx、超时和无新内容分类正确
- 单一来源失败不影响其他来源
- 日志和 API 不包含来源密钥或完整原始响应

## 提交边界

- test: 定义 Connector 与增量采集契约
- impl: 实现查询规划与运行持久化
- feat: 接入 RSS、Atom 与 Hacker News


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-16 | 初始查询规划与 RSS/HN 采集计划。 |
| v1.1 | 2026-07-16 | 对齐 Design-014 的 immutable published config、shared run/target、checkpoint 和来源安全契约；计划仍待完整独立审核。 |
