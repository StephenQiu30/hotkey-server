---
layer: Plan
doc_no: "28"
audience:
  - Tech-Lead
  - Dev
  - QA
  - Ops
feature_area: "area:n8n"
purpose: "创建 n8n 事实源和传播源采集 workflow 模板。"
canonical_path: "docs/plans/28-n8n事实源与传播源采集Workflow实现计划.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/24-n8n热点内容采集工作流PRD.md
  - docs/plans/25-n8n目录凭证与导入说明实现计划.md
  - docs/plans/27-n8n批量内容Ingest接口实现计划.md
outputs:
  - fact_source_collector workflow 模板
  - signal_source_collector workflow 模板
triggers:
  - "采集 workflow 节点或来源配置变更"
downstream:
  - docs/plans/29-n8n日报候选与日报保存实现计划.md
---

# 28-n8n事实源与传播源采集Workflow实现计划

## 1. 目标

提供两个可导入 workflow 模板，分别处理事实源采集和传播源采集，并统一调用后端批量 ingest API。

## 2. 文件清单

- `n8n/workflows/fact_source_collector.json`
- `n8n/workflows/signal_source_collector.json`
- `n8n/README.md`

## 3. 任务拆解

- 创建事实源 workflow：定时触发、读取来源配置、HTTP/RSS 获取、字段标准化、调用 ingest。
- 创建传播源 workflow：定时触发、读取来源配置、HTTP/RSS 获取、字段标准化、调用 ingest。
- 为每次调用设置 workflowName、executionId、Idempotency-Key。
- 成功和失败都调用 workflow 状态回写。

## 4. TDD 与验证

- workflow JSON 可导入 n8n。
- workflow 模板不包含真实凭证。
- 模板中 internal API URL、header 和 body 与后端契约一致。

## 5. 执行顺序

1. 先完成后端批量 ingest。
2. 再生成 workflow 模板。
3. 最后用本地 n8n 或 JSON 静态检查验证。

## 6. 回滚策略

- workflow 模板可单独回滚，不影响后端 API。
- 单个来源失败不阻断整个采集链路。

## 7. 验收命令

```bash
rg -n "HOTKEY_INTERNAL_API_KEY|SMTP_PASSWORD|OPENAI_API_KEY" n8n || true
```

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-27 | StephenQiu30 | 1.0.0 | 初版，按 n8n 采集工作流 PRD 拆分 |
