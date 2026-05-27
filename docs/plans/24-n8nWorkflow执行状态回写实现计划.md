---
layer: Plan
doc_no: "24"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:n8n"
purpose: "实现 n8n workflow 成功、失败和错误信息回写能力。"
canonical_path: "docs/plans/24-n8nWorkflow执行状态回写实现计划.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/23-n8n外部自动化编排接入PRD.md
  - docs/plans/23-n8nInternalAPI鉴权与幂等实现计划.md
outputs:
  - workflow execution 状态模型
  - workflow execution 回写接口
triggers:
  - "workflow 状态字段或错误回写规则变更"
downstream:
  - docs/plans/25-n8n目录凭证与导入说明实现计划.md
---

# 24-n8nWorkflow执行状态回写实现计划

## 1. 目标

提供 n8n 执行状态回写接口，使每次 workflow 执行都能在后端留下可排查记录。

## 2. 文件清单

- `internal/httpapi/router.go`
- `internal/httpapi/*_test.go`
- `internal/openapi/spec.go`
- `db/schema.sql`

## 3. 任务拆解

- 设计 workflow execution 请求字段：workflowName、executionId、tenantId、status、runStartedAt、runFinishedAt、errorMessage、metadata。
- 实现 `POST /api/v1/internal/workflows/n8n/executions`。
- 实现错误分支可用的 `POST /api/v1/internal/workflows/n8n/errors`，或在同一接口中用 status 表达失败。
- 记录关联 runId、reportId 或 collectorRunId。
- OpenAPI 中明确成功、失败和鉴权错误响应。

## 4. TDD 与验证

- 成功状态可写入并返回记录 ID。
- 失败状态必须保留错误信息。
- 重复 executionId 加幂等键不产生重复记录。
- 鉴权失败不可写入。

## 5. 执行顺序

1. 先定义请求/响应测试。
2. 再实现 service 和 HTTP handler。
3. 最后补 OpenAPI 和 schema 注释。

## 6. 回滚策略

- 回滚接口时不影响已有业务 API。
- 状态表字段新增必须兼容空库重建。

## 7. 验收命令

```bash
go test ./internal/httpapi ./internal/openapi
go test ./...
```

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-27 | StephenQiu30 | 1.0.0 | 初版，按 n8n 编排接入 PRD 拆分 |
