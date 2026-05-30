---
layer: PRD
doc_no: "05"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:queue"
purpose: "定义单 server 内部 scheduler、Redis 队列、任务幂等和重试机制。"
canonical_path: "docs/product/prd/05-Redis任务队列与调度PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "Redis任务队列与调度需求边界"
  - "Redis任务队列与调度TDD验收标准"
triggers:
  - "Redis任务队列与调度范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/05-Redis任务队列与调度实现计划.md"
---

# 05-Redis任务队列与调度 PRD

## 1. 背景

单 server 内部需要 scheduler 和 Redis 队列支撑小时采集、embedding、聚类、日报和邮件发送。

## 2. 目标

定义 Redis 队列、任务类型、幂等、重试、dead letter 和任务审计。

## 3. 范围

- Go 内部 scheduler。
- Redis queue producer/consumer。
- 任务类型和 payload schema。
- `jobs` 持久化审计。
- retry/backoff/dead letter。

## 4. 非目标

- 不引入 Temporal/Asynq。
- 不拆成多个服务部署。
- 不实现具体业务任务逻辑。

## 5. 用户故事

- 作为系统，我可以每小时自动创建采集任务。
- 作为管理员，我可以查看任务状态和失败原因。
- 作为 worker，我可以幂等消费任务并安全重试。

## 6. 数据与 API 边界

数据表：`jobs`。

API：管理员查看队列状态、任务详情、重试失败任务。

## 7. 后台任务影响

任务类型：`collect_source`、`normalize_item`、`generate_embedding`、`cluster_hotspots`、`score_hotspots`、`generate_report`、`send_daily_email`、`refresh_rss_cache`。

## 8. 配置影响

- `HOTKEY_REDIS_URL`
- `HOTKEY_RUNTIME_MODE=all|api|worker`
- scheduler tick interval

## 9. 错误与降级

Redis 不可用时 API 可启动但 worker unhealthy，任务不应静默丢失。

## 10. 安全与合规

任务 payload 不存明文 secret，不写入用户敏感邮件内容正文之外的额外隐私数据。

## 11. 验收标准

- Given enabled source，When scheduler tick，Then 创建幂等采集任务。
- Given 同一 idempotency key，When 重复入队，Then 只保留一个待执行任务。
- Given 可重试错误，When worker 失败，Then attempt 增加并按 backoff 重试。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/05-Redis任务队列与调度PRD.md
2. Read Plan: docs/plans/05-Redis任务队列与调度实现计划.md
3. Write failing test first
4. Run expected failing command
5. Implement minimal code
6. Run required verification
7. Update OpenAPI or migrations when needed
8. Commit with Chinese message
9. Report commands, results, risks, and changed files back to Linear
```

Symphony 在本地 `Agents` 目录监听 Linear issue，并在独立 workspace 中执行。HotKey 不重写 Symphony 规范，只在 `WORKFLOW.md` prompt 中约束执行行为。

## 14. PRD 自审清单

- 本 PRD 是否只覆盖一个 feature。
- 用户、管理员或系统任务的输入输出是否明确。
- 范围和非目标是否能阻止越界实现。
- 数据、API、任务和配置影响是否可拆成 Plan。
- 验收标准是否可测试、可自动化、可在 harness 中执行。
- 是否遵循 TDD，且不要求先写生产代码。

## 15. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-31 | StephenQiu30 | 1.0.0 | 初版，按 server-only AI 热点检测与日报服务 feature 拆分创建 |

