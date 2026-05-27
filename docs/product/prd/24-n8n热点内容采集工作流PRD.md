---
layer: PRD
doc_no: "24"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:n8n"
purpose: "定义 n8n 热点内容采集工作流的来源分层、内容标准化、批量写入和可靠性边界。"
canonical_path: "docs/product/prd/24-n8n热点内容采集工作流PRD.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/engineering/2-n8n外部自动化编排与AI热点日报工作流设计.md
  - docs/product/prd/4-来源与采集合规PRD.md
  - docs/product/prd/5-内容标准化与去重PRD.md
outputs:
  - n8n 热点内容采集工作流需求边界
  - 事实源与传播源采集验收标准
triggers:
  - "外部来源、采集字段或来源可靠性规则变更"
  - "对应 issue 拆分或合并"
downstream:
  - docs/plans/26-n8n来源分层与采集Payload实现计划.md
  - docs/plans/27-n8n批量内容Ingest接口实现计划.md
  - docs/plans/28-n8n事实源与传播源采集Workflow实现计划.md
---

# 24-n8n热点内容采集工作流 PRD

## 1. 背景

AI 热点监测需要同时关注官方事实源和社区传播源。n8n 负责定时访问外部来源、做轻量标准化和调用后端 ingest；hotkey-server 负责来源校验、去重、内容入库、可信度和后续事件聚合。

## 2. 目标

- 支持 n8n 按事实源和传播源分层采集 AI 热点内容。
- 定义统一 ingest payload，保证标题、URL、发布时间、来源类型、语言和原始元数据可追踪。
- 支持批量写入，返回 accepted、created、duplicated、rejected 等结果。
- 保证来源可靠性边界清晰：事实源用于确认事件，传播源用于热度参考。

## 3. 范围

- 第一阶段优先覆盖 RSS、公开网页和手工配置来源。
- 内容字段以日报和事件聚类需要为准，不追求全平台抓取。
- n8n workflow 只做标准化和转发，不做事实裁决。
- hotkey-server 记录采集运行结果和失败原因。

## 4. 非目标

- 不做无授权全平台抓取。
- 不在第一阶段实现浏览器自动登录、验证码绕过或付费内容抓取。
- 不要求分钟级实时；采集频率可按来源实际更新频率配置。
- 不由 n8n 直接计算事件可信度或最终热点排名。

## 5. 数据与 API 边界

- 输入来源必须映射到后端 `sources` 或租户来源配置。
- 批量 ingest API 必须支持 sourceCode、sourceType、items、workflowName、executionId。
- 每条内容必须保留 canonical URL、content hash 或等价去重依据。
- 失败项不能阻塞同批次其他有效内容入库。

## 6. 验收标准

- n8n 可以导入事实源和传播源采集 workflow 模板。
- workflow 调用后端批量 ingest API 后，后端返回创建、重复、拒绝统计。
- 来源不存在、来源禁用或字段不完整时，后端返回明确错误或 rejected 结果。
- workflow JSON 中不包含真实密钥。
- `go test ./...` 通过。

## 7. 风险与降级

- 外部来源不可用时记录失败，不影响其他来源采集。
- 内容解析失败时保留原始 URL 和错误原因，避免误入库。
- pgvector 不可用时，内容入库仍可完成，后续聚类降级处理。

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-27 | StephenQiu30 | 1.0.0 | 初版，按 n8n 内容采集功能闭环创建 |
