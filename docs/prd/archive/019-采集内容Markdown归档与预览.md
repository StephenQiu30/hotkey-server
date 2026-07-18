---
layer: PRD
prd_no: "019"
doc_no: "019"
title: 采集内容Markdown归档与预览
audience: [PM, Dev, QA, Ops]
feature_area: 内容归档与阅读
purpose: 交付授权 Feed 内容的 Markdown 归档和安全读取 API
phase: P1
priority: P0
status: accepted
execution_status: done
version: v1.1
owner: HotKey Server Team
depends_on: [PRD-006, PRD-007, PRD-017]
design_refs:
  - docs/design/archive/016-采集内容Markdown归档与预览设计.md
canonical_path: docs/prd/archive/019-采集内容Markdown归档与预览.md
inputs:
  - docs/design/archive/016-采集内容Markdown归档与预览设计.md
outputs:
  - Markdown 归档生成与读取 API
  - 供 Web 生成客户端消费的 OpenAPI 契约
triggers:
  - Design-016 accepted
downstream:
  - docs/plans/archive/019-采集内容Markdown归档与预览计划.md
  - docs/acceptance/archive/019-采集内容Markdown归档与预览验收.md
---

# 采集内容 Markdown 归档与预览

## 目标

向下游提供真实、可校验的来源 Feed 正文/摘要 Markdown 归档，并对历史未捕获内容稳定返回 `not_captured`。

## 范围

1. RSS/Atom body 优先使用 `content`，退回 description/summary；只在 `allow_body_storage=true` 时保存。
2. 使用 `html-to-markdown/v2@v2.5.2` 在 Ingestion 内生成 Markdown，复用现有 MinIO text evidence asset，不改 Schema。
3. 新增 `GET /api/v1/contents/{id}/document`，返回 `ready | not_captured` 和安全 metadata/Markdown，不暴露对象存储实现。
4. Swagger 注解生成下游预览所需契约，不暴露 HTML/PDF/object key。

## 非范围

- canonical 网页抓取、论文 PDF 解析、JS 渲染、Jina Reader 或 Readeck Readability。
- 服务端 PDF 产物、Gotenberg、PDF 缓存/重建任务。
- 历史无 body 数据回填或替换已发布来源配置。
- Web 页面、来源表单、工作台布局和 print CSS；它们由 `../hotkey-web` 自身文档链交付。

## 验收标准

1. HTML 强调、列表、链接可确定性转为 Markdown，不发起额外网络请求；RSS content 优先行为有 fixture。
2. 未授权的 body 不持久化；无 asset 返回 `not_captured`，有 asset 且完整性正确返回 `ready`。
3. 非法 id 返回 `invalid_request`/400，缺失或已删除内容返回 `not_found`/404，MinIO 不可用、过大对象、SHA/大小不匹配返回 `unavailable`/503；均不泄露 object key、endpoint 或正文。
4. OpenAPI 由 Server 注解生成并通过漂移校验，应用层无手写重复契约。
5. 并发读、重放写入、事务失败补偿、删除/delete_pending、孤儿对账和 MinIO 故障恢复均有可执行验收。
6. Server `make ci && make clean` 通过，`db/schema.sql` 零差异。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-07-18 | 建立交付范围和可执行验收，等待独立审核。 |
| v0.2 | 2026-07-18 | 按 Plan Review 冻结 API 业务码/HTTP 映射。 |
| v0.3 | 2026-07-18 | 将 Web 交付移出 Server PRD，补齐证据生命周期验收。 |
| v0.4 | 2026-07-18 | 移除跨仓客户端生成验收，仅保留 Server 注解与 OpenAPI 门禁。 |
| v1.0 | 2026-07-18 | 经独立复核通过，PRD accepted/ready。 |
| v1.1 | 2026-07-18 | PLAN-019 实现、全量门禁与独立复审通过，执行状态更新为 done。 |
