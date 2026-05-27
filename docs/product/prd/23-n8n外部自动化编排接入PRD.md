---
layer: PRD
doc_no: "23"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:n8n"
purpose: "定义 n8n 作为外部自动化编排层接入 hotkey-server 的边界、认证、幂等和执行状态回写能力。"
canonical_path: "docs/product/prd/23-n8n外部自动化编排接入PRD.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/engineering/2-n8n外部自动化编排与AI热点日报工作流设计.md
outputs:
  - n8n 外部自动化编排接入需求边界
  - n8n internal API 与 workflow 状态回写验收标准
triggers:
  - "n8n 接入方式、鉴权方式或执行状态模型变更"
  - "对应 issue 拆分或合并"
downstream:
  - docs/plans/23-n8nInternalAPI鉴权与幂等实现计划.md
  - docs/plans/24-n8nWorkflow执行状态回写实现计划.md
  - docs/plans/25-n8n目录凭证与导入说明实现计划.md
---

# 23-n8n外部自动化编排接入 PRD

## 1. 背景

HotKey 需要通过 n8n 执行定时采集、AI 处理编排和邮件发送，但事实写入、租户边界、去重、审计和状态记录必须留在 `hotkey-server`。本 PRD 只定义 n8n 进入后端的统一接入能力，不直接实现具体采集源或日报内容生成。

## 2. 目标

- 为 n8n 提供受控的 internal API 入口。
- 支持 shared key 鉴权、租户上下文和幂等键，避免 workflow 重试造成重复写入。
- 记录 workflow 执行成功、失败、错误信息和关联业务对象。
- 提供仓库内 n8n 模板目录和导入说明，但不保存真实凭证。

## 3. 范围

- `X-HotKey-Internal-Key`、`X-HotKey-Tenant-ID`、`Idempotency-Key` 的校验规则。
- workflowName、executionId、tenantId 的统一请求字段。
- workflow execution 成功和失败回写接口。
- n8n 目录、workflow 模板存放规则和凭证配置说明。

## 4. 非目标

- 不让 n8n 直接连接 PostgreSQL、Redis 或 pgvector。
- 不在本 PRD 中实现全部外部来源采集逻辑。
- 不在本 PRD 中实现复杂消息队列、queue mode 或多 worker 部署。
- 不提交 SMTP、AI Provider、n8n API Key 等真实密钥。

## 5. 数据与 API 边界

- n8n 只能通过 `/api/v1/internal/*` 写入或回写状态。
- hotkey-server 负责鉴权、租户校验、幂等、审计和错误响应。
- workflow 执行状态应能关联到采集 run、日报 run 或后续任务 run。
- OpenAPI 必须覆盖 internal API 的请求、响应和错误结构。

## 6. 验收标准

- 未携带或携带错误 internal key 的请求被拒绝。
- 相同 `Idempotency-Key` 的重试不会重复生成业务写入。
- workflow 成功和失败状态都能被后端记录并查询或用于排查。
- n8n 模板目录存在导入说明，且不包含真实凭证。
- `go test ./...` 通过。

## 7. 风险与降级

- 如果 n8n 不可用，后端已有数据和日报查询不受影响。
- 如果 workflow 状态回写失败，n8n 可按幂等键重试。
- 如果 internal key 泄露，必须支持通过环境变量轮换。

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-27 | StephenQiu30 | 1.0.0 | 初版，按 n8n 编排接入功能闭环创建 |
