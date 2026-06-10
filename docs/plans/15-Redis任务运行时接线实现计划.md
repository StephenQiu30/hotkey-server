---
layer: Plan
doc_no: "15"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:queue"
purpose: "将 Redis 队列与 job 状态管理接入统一运行时，为后续业务任务提供可恢复、可审计的异步执行基础。"
canonical_path: "docs/plans/15-Redis任务运行时接线实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/design/002-热点平台能力补齐与上线接通设计.md"
  - "docs/plans/14-runtime服务装配实现计划.md"
  - "docs/engineering/1-Go后端重建与开源仓库治理设计.md"
  - "docs/plans/05-Redis任务队列与调度实现计划.md"
outputs:
  - "Redis 任务运行时接线实现任务"
triggers:
  - "队列、重试或 job 状态模型变化"
downstream:
  - "docs/plans/16-X来源采集主线接通实现计划.md"
---

# 15-Redis任务运行时接线实现计划

## 1. 目标

将 Redis 队列与 job 状态管理接入统一运行时，为后续业务任务提供可恢复、可审计的异步执行基础。

## 2. 文件清单

- 修改：`internal/queue/queue.go`
- 修改：`internal/repository/postgres/jobrepo/`
- 修改：`internal/worker/worker.go`
- 修改：`internal/app/server.go`

## 3. 任务拆解

1. 为入队幂等、重试和 Redis 不可用路径写失败测试。
2. 统一 queue 和 job repository 的运行时接线。
3. 完善重试、失败和 dead letter 状态记录。
4. 确认 worker 可消费统一 job 模型。
5. 运行验证并回写结果。

## 4. TDD 与验证

- job 具备幂等、重试和失败追踪能力。
- Redis 不可用时行为明确降级，不静默丢任务。
- worker 消费路径由集成测试覆盖。

## 5. 执行顺序

1. `test:` 入队幂等、重试、Redis 降级失败测试。
2. `impl:` queue 与 jobrepo 运行时接线。
3. `refactor:` 统一 job 状态枚举与记录方式。

## 6. 回滚策略

回滚 runtime 接线，保留 queue 与 jobrepo 独立实现；已入队任务需人工清理或等待 TTL 过期。

## 7. 验收命令

```bash
go test ./internal/queue/...
go test ./internal/repository/postgres/jobrepo/...
gofmt -w cmd internal
go test ./...
python3 -m unittest discover -s tests
```

## 8. Symphony / Linear 要求

任务状态、标签和流转规则完全以本仓库 `WORKFLOW.md` 和本地 Symphony 实现为准。Plan 不定义额外状态、不发明额外标签、不覆盖 Symphony 的状态机。

Linear issue 只承载任务内容：PRD 路径、Plan 路径、任务范围、禁止范围、TDD 验收命令和回写要求。Symphony 负责监听 active states、创建 workspace、运行 Codex，并按 `WORKFLOW.md` prompt 驱动执行。

## 9. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-06-09 | StephenQiu30 | 1.0.0 | 按文档规范重写，编号调整为 15 |
