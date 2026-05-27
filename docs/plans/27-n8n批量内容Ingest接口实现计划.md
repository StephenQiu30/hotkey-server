---
layer: Plan
doc_no: "27"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:n8n"
purpose: "实现 n8n 采集内容批量写入 hotkey-server 的 internal ingest API。"
canonical_path: "docs/plans/27-n8n批量内容Ingest接口实现计划.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/24-n8n热点内容采集工作流PRD.md
  - docs/plans/23-n8nInternalAPI鉴权与幂等实现计划.md
  - docs/plans/26-n8n来源分层与采集Payload实现计划.md
outputs:
  - n8n 批量内容 ingest API
  - ingest 结果统计测试证据
triggers:
  - "批量 ingest 接口或去重规则变更"
downstream:
  - docs/plans/28-n8n事实源与传播源采集Workflow实现计划.md
---

# 27-n8n批量内容Ingest接口实现计划

## 1. 目标

提供 `POST /api/v1/internal/ingest/contents`，让 n8n 可以批量提交标准化后的热点内容。

## 2. 文件清单

- `internal/content/service.go`
- `internal/httpapi/router.go`
- `internal/httpapi/*_test.go`
- `internal/openapi/spec.go`
- `db/schema.sql`

## 3. 任务拆解

- 设计批量请求结构：workflowName、executionId、tenantId、sourceCode、sourceType、items。
- 每条 item 调用后端内容标准化与去重能力。
- 返回 accepted、created、duplicated、rejected、runId。
- 对单条失败做 rejected，不阻断同批次其他内容。
- 记录采集运行结果，便于 n8n 回写和排查。

## 4. TDD 与验证

- 有效批量请求返回统计。
- 重复 URL 或内容 hash 计入 duplicated。
- 无效 item 计入 rejected，并包含原因。
- 鉴权失败不会入库。

## 5. 执行顺序

1. 先写 HTTP 测试。
2. 再扩展 content service 或新增 internal ingest service。
3. 最后补 OpenAPI 和 schema 注释。

## 6. 回滚策略

- 保留已有 admin 单条 ingest API。
- 批量接口失败时 n8n 可以按幂等键重试。

## 7. 验收命令

```bash
go test ./internal/content ./internal/httpapi ./internal/openapi
go test ./...
```

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-27 | StephenQiu30 | 1.0.0 | 初版，按 n8n 采集工作流 PRD 拆分 |
