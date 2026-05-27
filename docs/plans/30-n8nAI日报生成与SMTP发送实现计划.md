---
layer: Plan
doc_no: "30"
audience:
  - Tech-Lead
  - Dev
  - QA
  - Ops
feature_area: "area:n8n"
purpose: "创建 n8n AI 日报生成和 SMTP 邮件发送 workflow 模板，并完成状态回写。"
canonical_path: "docs/plans/30-n8nAI日报生成与SMTP发送实现计划.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/25-n8nAI热点日报邮件工作流PRD.md
  - docs/plans/29-n8n日报候选与日报保存实现计划.md
outputs:
  - daily_ai_hotspot_email_digest workflow 模板
  - SMTP 邮件发送验收证据
triggers:
  - "日报邮件模板、AI 节点或 SMTP 配置变更"
downstream:
  - docs/acceptance/
---

# 30-n8nAI日报生成与SMTP发送实现计划

## 1. 目标

提供每日 AI 热点日报 workflow：获取后端候选、生成 Markdown/HTML、保存日报、通过 SMTP 发送邮件并回写状态。

## 2. 文件清单

- `n8n/workflows/daily_ai_hotspot_email_digest.json`
- `n8n/README.md`
- `.env.example`

## 3. 任务拆解

- 创建 Schedule Trigger，默认每日执行前一天日报。
- 调用 daily-candidates 获取后端候选。
- 使用 AI 节点或模板节点生成 Markdown 和 HTML。
- 调用 daily 保存接口，先保存日报。
- 使用 SMTP 节点发送邮件。
- 成功或失败均调用 workflow 状态回写接口。

## 4. TDD 与验证

- workflow JSON 可导入 n8n。
- SMTP 凭证使用 n8n Credentials，不进入仓库。
- 后端日报保存成功但 SMTP 失败时，workflow 仍回写失败状态。
- 邮件正文包含事实源和传播源链接。

## 5. 执行顺序

1. 先完成日报候选和保存接口。
2. 再创建 workflow 模板。
3. 最后做本地或测试 SMTP 演练。

## 6. 回滚策略

- workflow 可单独禁用，不影响后端日报查询。
- AI 生成失败时可降级为规则模板正文。

## 7. 验收命令

```bash
go test ./...
rg -n "SMTP_PASSWORD|OPENAI_API_KEY|HOTKEY_INTERNAL_API_KEY" n8n || true
```

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-27 | StephenQiu30 | 1.0.0 | 初版，按 n8n 日报邮件 PRD 拆分 |
