---
layer: Acceptance
doc_no: "019"
audience: [Dev, QA, Ops]
feature_area: 内容归档与阅读
purpose: 验收授权 Feed 内容的 Markdown 归档与安全读取 API
canonical_path: docs/acceptance/019-采集内容Markdown归档与预览验收.md
status: review
conclusion: pending
version: v0.3
owner: HotKey Server Team
inputs:
  - docs/design/016-采集内容Markdown归档与预览设计.md
  - docs/prd/019-采集内容Markdown归档与预览.md
  - docs/plans/019-采集内容Markdown归档与预览计划.md
outputs:
  - PLAN-019 长期验收结论
triggers:
  - PLAN-019 开始实施
downstream:
  - docs/operations/README.md
---

# 采集内容 Markdown 归档与预览验收

## 实施前标准审核

| 项目 | 状态 | 证据 |
|---|---|---|
| 授权 Feed body 与历史无 body 边界 | passed | 独立 Plan Review APPROVED |
| Markdown 转换、网络非范围与安全渲染 | passed | 独立 Plan Review APPROVED |
| ready/not_captured、存储故障与完整性 | passed | 独立 Plan Review APPROVED |
| Result、稳定错误码与注解生成 OpenAPI | passed | 独立 Plan Review APPROVED |
| 并发、幂等、补偿、删除、对账与恢复 | passed | 独立 Plan Review APPROVED |

## RED 证据

待实施。必须记录 Feed/转换、归档生命周期与 document HTTP 契约三个可执行红灯，不接受只有静态分析的伪红灯。

## GREEN 证据

| 类型 | 命令/证据 | 状态 |
|---|---|---|
| Server 定向 | 待填 | pending |
| Server 全量 | `make ci && make clean` | pending |
| OpenAPI | 生成产物与注解一致 | pending |
| Schema | `git diff --exit-code -- db/schema.sql` | pending |
| 独立复审 | 逐项 passed/failed/blocked | pending |

## 数据与外部依赖限制

当前开发数据中来源均未允许 body 归档，旧 Content 无可恢复正文。验收只能用明确 `allow_body_storage=true` 的可丢弃 fixture 证明 ready 路径，用现有数据证明 not_captured 路径。无外部 MinIO 时可以用协议 fixture 验证，但必须把实机 MinIO 标记为未执行风险。

## 结论

`pending`：实施和证据尚未完成。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-18 | 创建实施前验收模板，结论 pending。 |
| v0.2 | 2026-07-18 | 分离 Web 页面验收，补齐 Server 生命周期、错误和 Schema 门禁。 |
| v0.3 | 2026-07-18 | 记录实施前标准经非主要编写者审核通过；实现证据仍 pending。 |
