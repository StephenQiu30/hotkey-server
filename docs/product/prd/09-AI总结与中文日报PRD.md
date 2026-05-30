---
layer: PRD
doc_no: "09"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:report"
purpose: "定义 Qwen 中文摘要、时间线、影响分析和日报生成。"
canonical_path: "docs/product/prd/09-AI总结与中文日报PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "AI总结与中文日报需求边界"
  - "AI总结与中文日报TDD验收标准"
triggers:
  - "AI总结与中文日报范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/09-AI总结与中文日报实现计划.md"
---

# 09-AI总结与中文日报 PRD

## 1. 背景

日报需要把热点转换成中文摘要、时间线和影响分析，方便用户快速阅读。

## 2. 目标

使用 Qwen 生成中文日报内容，并确保所有 AI 输出有来源引用。

## 3. 范围

- 热点中文摘要。
- 时间线。
- 影响分析。
- 频道日报和用户个性化日报。
- AI 失败降级。

## 4. 非目标

- 不实现 RSS XML 输出。
- 不实现 SMTP 发送。
- 不让 AI 直接决定热点排序。

## 5. 用户故事

- 作为用户，我每天收到中文 AI 热点摘要。
- 作为用户，我可以看到热点为什么重要。
- 作为系统，我在证据不足时不生成强观点。

## 6. 数据与 API 边界

数据表：`ai_summaries`、`daily_reports`。

API：日报查询、管理员重跑日报。

## 7. 后台任务影响

`generate_report` 读取热点和 source refs，调用 Qwen，保存 daily_reports，并触发邮件任务。

## 8. 配置影响

- `HOTKEY_DASHSCOPE_API_KEY`
- Qwen model
- prompt version

## 9. 错误与降级

DashScope 失败时生成规则版日报：标题、来源列表、基础摘要。

## 10. 安全与合规

AI prompt 必须包含来源引用约束，输出保存 `source_refs_json`。

## 11. 验收标准

- Given 热点簇和来源证据，When 生成日报，Then daily_report 为中文。
- Given 证据不足，When 生成日报，Then 不输出影响分析。
- Given Qwen 配置缺失，Then 任务状态为 `failed_config` 或生成降级日报。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/09-AI总结与中文日报PRD.md
2. Read Plan: docs/plans/09-AI总结与中文日报实现计划.md
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

