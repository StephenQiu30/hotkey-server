---
layer: PRD
doc_no: "20"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:realtime"
purpose: "支持实时流、Webhook 或授权平台推送，实现秒级或近秒级热点检测。"
canonical_path: "docs/product/prd/20-秒级实时检测PRD.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/engineering/1-Go后端重建与开源仓库治理设计.md
outputs:
  - 秒级实时检测需求边界
  - 秒级实时检测验收标准
triggers:
  - "秒级实时检测范围变更"
  - "对应 issue 拆分或合并"
downstream:
  - docs/plans/20-秒级实时检测实现计划.md
---

# 20-秒级实时检测 PRD

## 1. 背景

本 PRD 属于 HotKey Go 后端全面重构的新编号体系，阶段为 **P3 高级实时与事件图谱**。旧 FastAPI 实现、旧编号计划和一次性中间记录不作为本需求事实源。

## 2. 目标

支持实时流、Webhook 或授权平台推送，实现秒级或近秒级热点检测。

## 3. 范围

- 围绕 `秒级实时检测` 定义后端能力、数据边界、API 影响和验收口径。
- 与同编号 Plan 配对推进，实施前不得绕过本文档。
- 变更必须同步 GitHub issue、Linear issue 和 OpenAPI 影响说明。

## 4. 非目标

- 不恢复旧 FastAPI 目录、旧数据库结构或旧 OpenAPI 契约。
- 不保留一次性过程记录作为长期事实源。
- 不引入与本编号能力无关的跨阶段实现。

## 5. 数据与 API 边界

- 数据模型以 Go 重构后的 PostgreSQL schema 为事实源。
- API 以 `hotkey-server` 导出的 OpenAPI 为事实源。
- 需要端侧消费的字段必须先进入后端 schema、测试和 OpenAPI。

## 6. 验收标准

- 授权实时源可低延迟进入事件候选，并具备限流、熔断和降级。
- PRD 与 Plan 编号一致。
- 对应 GitHub/Linear issue 负责人明确。
- 无占位标记或未定稿提示。

## 7. 风险与降级

- 如果实现依赖外部平台授权，必须提供禁用和降级路径。
- 如果实现依赖 AI、pgvector 或 Redis，必须记录失败状态并保留可读历史结果。
- 如果范围跨阶段，必须拆分到后续编号，不在本任务中隐式完成。

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-26 | StephenQiu30 | 1.0.0 | 初版，按 Go 后端全面重构新编号体系创建 |
