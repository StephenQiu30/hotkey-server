---
layer: PRD
doc_no: "25"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:n8n"
purpose: "定义 n8n AI 热点日报邮件工作流的日报候选、AI 表达生成、SMTP 发送和状态回写能力。"
canonical_path: "docs/product/prd/25-n8nAI热点日报邮件工作流PRD.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/engineering/2-n8n外部自动化编排与AI热点日报工作流设计.md
  - docs/product/prd/9-日报生成PRD.md
outputs:
  - n8n AI 热点日报邮件工作流需求边界
  - 日报生成、邮件发送和状态回写验收标准
triggers:
  - "日报候选、邮件模板或发送渠道变更"
  - "对应 issue 拆分或合并"
downstream:
  - docs/plans/29-n8n日报候选与日报保存实现计划.md
  - docs/plans/30-n8nAI日报生成与SMTP发送实现计划.md
---

# 25-n8nAI热点日报邮件工作流 PRD

## 1. 背景

平台需要每天汇总前一天 AI 热点事件，并通过邮件发送给维护者或订阅用户。n8n 适合作为定时触发、AI 文案整理和 SMTP 发送的外部编排层；后端负责决定候选事件、保存日报和记录状态。

## 2. 目标

- 后端提供日报候选接口，返回结构化热点事件和证据链接。
- n8n 基于后端候选生成 Markdown 和 HTML 日报正文。
- n8n 通过 SMTP 发送邮件，并把成功或失败状态回写 hotkey-server。
- 日报先保存到后端，再发送邮件，保证邮件失败时仍可追溯。

## 3. 范围

- 第一阶段只实现每日 AI 热点日报，不实现复杂订阅偏好和多渠道推送。
- 邮件内容包含热点标题、摘要、事实源证据、传播源参考、可信度提示和来源链接。
- 后端保存最终 Markdown、HTML、结构化 JSON 和发送状态。
- n8n 负责表达整理，不负责改写事实归属。

## 4. 非目标

- 不把邮件系统做成完整营销邮件平台。
- 不在第一阶段实现复杂用户分群、退订、计费和多模板运营后台。
- 不让 AI 节点单独决定事件是否真实。
- 不提交真实 SMTP 密钥。

## 5. 数据与 API 边界

- `daily-candidates` 由后端返回，n8n 只消费候选。
- `daily` 保存接口接收 Markdown、HTML、结构化 JSON、reportDate、tenantId 和 workflow 信息。
- SMTP 发送结果通过 workflow execution 或 report delivery 状态回写。
- 邮件正文必须保留来源 URL，便于人工复核。

## 6. 验收标准

- n8n 可通过后端接口获取某天日报候选。
- workflow 可生成 Markdown 和 HTML 日报，并调用后端保存。
- SMTP 发送成功后回写成功状态；发送失败后回写失败状态和错误信息。
- 日报保存成功但邮件失败时，后端仍可查询到日报内容。
- workflow JSON 中不包含真实 SMTP 凭证。
- `go test ./...` 通过。

## 7. 风险与降级

- AI 生成失败时可使用后端结构化候选生成规则模板日报。
- SMTP 失败不影响日报保存。
- 候选事件为空时发送空日报或跳过发送必须由配置控制，并记录执行状态。

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-27 | StephenQiu30 | 1.0.0 | 初版，按 n8n 日报邮件功能闭环创建 |
