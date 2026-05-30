---
layer: Plan
doc_no: "05"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:queue"
purpose: "实现单 server 内部 scheduler、Redis 队列、任务幂等与重试。"
canonical_path: "docs/plans/05-Redis任务队列与调度实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/05-Redis任务队列与调度PRD.md"
outputs:
  - "Redis任务队列与调度实现任务"
triggers:
  - "任务类型或调度策略变化"
downstream:
  - "由 WORKFLOW.md 指定的 Symphony / Linear 流程接管"
---

# 05-Redis任务队列与调度实现计划

## 1. 目标

在单一 server 进程中提供 scheduler、Redis 队列和 worker runtime。

## 2. 文件清单

- 创建：`migrations/000005_jobs.up.sql`
- 创建：`internal/platform/redis/`
- 创建：`internal/queue/`
- 创建：`internal/scheduler/`
- 创建：`internal/worker/`
- 修改：`internal/app/server.go`
- 修改：`internal/config/config.go`

## 3. 任务拆解

1. 创建 `jobs` 审计表。
2. 定义 job types 和 payload schema。
3. 实现 Redis queue producer/consumer。
4. 实现幂等 key、attempt、retry/backoff、dead letter。
5. 实现 scheduler 每小时入队 `collect_source`。
6. 支持 `HOTKEY_RUNTIME_MODE=all|api|worker`。

## 4. TDD 与验证

- fake Redis 或测试 Redis 覆盖入队、重复幂等、失败重试。
- scheduler 使用 fake clock 测试小时级入队。
- app runtime 测试不同启动模式。

## 5. 执行顺序

1. `test:` queue/scheduler 失败测试。
2. `impl:` Redis、queue、scheduler、worker runtime。
3. `refactor:` app lifecycle 和 shutdown。

## 6. 回滚策略

禁用 worker runtime，回滚 jobs migration，API runtime 可继续运行。

## 7. 验收命令

```bash
go test ./...
python3 -m unittest discover -s tests
```

## 8. Symphony / Linear 要求

任务状态、标签和流转规则完全以本仓库 `WORKFLOW.md` 和本地 Symphony 实现为准。Plan 不定义额外状态、不发明额外标签、不覆盖 Symphony 的状态机。

Linear issue 只承载任务内容：PRD 路径、Plan 路径、任务范围、禁止范围、TDD 验收命令和回写要求。Symphony 负责监听 active states、创建 workspace、运行 Codex，并按 `WORKFLOW.md` prompt 驱动执行。

## 9. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-31 | StephenQiu30 | 1.0.0 | 初版 |
