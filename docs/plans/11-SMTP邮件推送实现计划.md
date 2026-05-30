---
layer: Plan
doc_no: "11"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:mail"
purpose: "实现 SMTP 邮件日报、发送时间、重试和投递记录。"
canonical_path: "docs/plans/11-SMTP邮件推送实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "docs/product/prd/11-SMTP邮件推送PRD.md"
outputs:
  - "SMTP邮件推送实现任务"
triggers:
  - "邮件 provider 或发送策略变化"
downstream:
  - "由 WORKFLOW.md 指定的 Symphony / Linear 流程接管"
---

# 11-SMTP邮件推送实现计划

## 1. 目标

使用 SMTP 按全局默认时间或用户自定义时间发送每日中文日报。

## 2. 文件清单

- 创建：`migrations/000011_email_deliveries.up.sql`
- 创建：`internal/platform/mailer/`
- 创建：`internal/service/mail/`
- 创建：`internal/repository/postgres/mailrepo/`
- 创建：`internal/worker/handlers/email/`
- 修改：`internal/config/config.go`

## 3. 任务拆解

1. 创建 `email_deliveries`。
2. 实现 SMTP mailer interface 和 fake SMTP 测试。
3. 实现邮件模板。
4. scheduler 根据用户发送时间入队 `send_daily_email`。
5. worker 发送邮件并记录投递状态。

## 4. TDD 与验证

- fake SMTP server 收到日报邮件。
- SMTP 未配置时任务 `failed_config`。
- 用户关闭邮件时不入队发送任务。

## 5. 执行顺序

1. `test:` mail service、fake SMTP、scheduler 失败测试。
2. `impl:` migration、mailer、service、worker。
3. `refactor:` 邮件模板和配置整理。

## 6. 回滚策略

禁用邮件 worker，回滚 email_deliveries migration。

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
