---
layer: Plan
doc_no: 021
audience: Dev, QA, Ops
feature_area: Worker运行面收敛
purpose: 定义单实例 Worker 调度主线、统一任务执行入口、任务幂等与失败处理的落地步骤与门禁
canonical_path: docs/plans/021-Worker运行面收敛计划.md
status: review
version: v1.0
owner: Codex
inputs:
  - docs/design/011-接口契约与任务运行设计.md
  - docs/plans/018-后端工程化落地计划.md
outputs:
  - Worker 统一调度入口
  - 单实例边界与幂等策略
triggers:
  - 需要避免重复调度、重复通知和任务逻辑散落
downstream:
  - docs/acceptance/004-后端工程化架构验收.md
---

# 背景

Worker 已被定义为正式运行面，但如果不先收敛调度入口、单实例边界和幂等策略，后续任务代码会再次各写各的，并在部署时留下重复执行风险。

# 目标

1. 建立统一 scheduler / job runner 入口。
2. 明确任务必须通过 service 调用业务核心。
3. 明确单实例运行边界、任务幂等、失败记录和重试策略。

# 非目标

1. 本计划不引入分布式多副本调度系统。
2. 本计划不实现未来 leader election 方案。

# Task 1: 统一调度与执行入口

目标：

1. 建立 scheduler 注册入口。
2. 建立 job runner 执行入口。
3. 统一任务生命周期日志。

验证门禁：

```bash
go test ./internal/jobs/... ./internal/app/... ./...
```

# Task 2: 任务依赖边界收敛

目标：

1. 任务统一调用 service。
2. 禁止 job 直接堆积 SQL、外部 API 和业务判断。

验证门禁：

```bash
go test ./internal/jobs/... ./...
```

# Task 3: 单实例边界、幂等与失败处理

目标：

1. 明确单实例默认部署边界。
2. 关键任务具备去重键或幂等保护。
3. 失败原因、重试次数和审计记录可追踪。

验证门禁：

```bash
go test ./internal/jobs/... ./internal/... ./...
```

# 风险与边界

1. 若后续部署越过单实例边界，必须先补专门设计文档。
2. 若幂等与失败处理不先建，通知和导出类任务会有高副作用风险。

# 变更记录

## v1.0

1. 新建 Worker 运行面收敛计划。
2. 将调度入口、单实例边界和幂等策略单独收敛。
