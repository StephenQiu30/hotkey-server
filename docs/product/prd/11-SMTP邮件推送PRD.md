---
layer: PRD
doc_no: "11"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:mail"
purpose: "定义 SMTP 邮件日报、发送时间、重试和投递记录。"
canonical_path: "docs/product/prd/11-SMTP邮件推送PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "SMTP邮件推送需求边界"
  - "SMTP邮件推送TDD验收标准"
triggers:
  - "SMTP邮件推送范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/11-SMTP邮件推送实现计划.md"
---

# 11-SMTP邮件推送 PRD

## 1. 背景

用户需要通过邮件接收每日中文 AI 热点日报。第一版使用 SMTP。

## 2. 目标

实现 SMTP 配置、邮件日报任务、发送记录、失败重试和用户发送时间设置。

## 3. 范围

- SMTP mailer。
- 邮件模板。
- 管理员全局默认发送时间。
- 用户自定义发送时间。
- email_deliveries 记录。
- 失败重试。

## 4. 非目标

- 不接 Resend/SendGrid。
- 不做复杂营销邮件系统。
- 不做页面退订。

## 5. 用户故事

- 作为用户，我可以每天在指定时间收到中文 AI 日报。
- 作为用户，我可以关闭邮件推送。
- 作为管理员，我可以查看邮件发送失败原因。

## 6. 数据与 API 边界

数据表：`email_deliveries`，并复用 users 的邮箱、时区和发送时间。

API：用户邮件设置 API、管理员邮件投递记录 API。

## 7. 后台任务影响

`send_daily_email` 读取 daily_reports 和用户设置，通过 SMTP 发送并写投递记录。

## 8. 配置影响

- `HOTKEY_SMTP_HOST`
- `HOTKEY_SMTP_PORT`
- `HOTKEY_SMTP_USERNAME`
- `HOTKEY_SMTP_PASSWORD`
- `HOTKEY_SMTP_FROM`

## 9. 错误与降级

SMTP 未配置时邮件任务 disabled，RSS 不受影响。发送失败按 retry 策略重试，最终 dead letter。

## 10. 安全与合规

SMTP 密码只能来自环境变量或 secret，不入库。邮件必须提供禁用入口对应 API。

## 11. 验收标准

- Given 用户开启邮件，When 到发送窗口，Then 创建 send_daily_email 任务。
- Given fake SMTP server，When 任务执行，Then fake server 收到邮件。
- Given SMTP 配置缺失，When 发送任务执行，Then 标记 `failed_config`。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/11-SMTP邮件推送PRD.md
2. Read Plan: docs/plans/11-SMTP邮件推送实现计划.md
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

