---
layer: Plan
doc_no: "26"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:n8n"
purpose: "定义 n8n 采集工作流使用的事实源、传播源分层和统一 payload。"
canonical_path: "docs/plans/26-n8n来源分层与采集Payload实现计划.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/24-n8n热点内容采集工作流PRD.md
  - docs/product/prd/4-来源与采集合规PRD.md
outputs:
  - n8n 来源分层清单
  - n8n ingest payload 字段定义
triggers:
  - "采集来源或 payload 字段变更"
downstream:
  - docs/plans/27-n8n批量内容Ingest接口实现计划.md
---

# 26-n8n来源分层与采集Payload实现计划

## 1. 目标

明确 n8n 第一阶段采集哪些类型的来源，以及这些内容进入后端前必须具备哪些字段。

## 2. 文件清单

- `docs/engineering/2-n8n外部自动化编排与AI热点日报工作流设计.md`
- `internal/openapi/spec.go`
- `n8n/README.md`

## 3. 任务拆解

- 将来源分为事实源和传播源。
- 事实源优先：官方博客、官方新闻、论文页、GitHub Release。
- 传播源优先：Hacker News、Reddit、YouTube、Product Hunt、技术媒体 RSS。
- 定义统一 payload：sourceCode、sourceType、externalId、url、title、summary、contentText、language、region、publishedAt、rawPayload。
- 明确 sourceType 只能表达来源角色，不能替代可信度评分。

## 4. TDD 与验证

- payload 缺少 url、title、publishedAt 或 sourceCode 时被拒绝。
- sourceType 非 fact / propagation 时被拒绝。
- rawPayload 可为空，但字段结构必须可序列化。

## 5. 执行顺序

1. 先确定字段和枚举。
2. 再同步 OpenAPI schema。
3. 最后更新 n8n README 示例。

## 6. 回滚策略

- 字段新增应保持向后兼容。
- 删除字段必须同步 workflow 模板。

## 7. 验收命令

```bash
go test ./internal/openapi ./internal/httpapi
go test ./...
```

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-27 | StephenQiu30 | 1.0.0 | 初版，按 n8n 采集工作流 PRD 拆分 |
