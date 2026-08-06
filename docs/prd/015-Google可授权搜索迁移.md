---
layer: PRD
scope: backend
doc_no: "015"
title: Google可授权搜索迁移
status: draft
version: v1.0
owner: HotKey Team
phase: P2
canonical_path: docs/prd/015-Google可授权搜索迁移.md
design: docs/design/015-Google可授权搜索迁移设计.md
plan: docs/plans/015-Google可授权搜索迁移计划.md
---

# Google可授权搜索迁移

## 用户问题

参考项目包含 Google 搜索实现；Google Custom Search JSON API 已对新客户关闭，存量客户也将在 2027-01-01 停用。

## 产品目标

为存量 Custom Search 客户提供短期兼容，同时把新部署迁移到可获得授权的限定域搜索或官方全网搜索方案。

## 范围

- 存量 Custom Search 兼容连接器
- 弃用检测和迁移提示
- 限定域/全网搜索能力抽象
- 成本、配额和合同审计

## 用户故事

- 作为编辑者，我能选择真实可用的来源能力并预览查询结果。
- 作为管理员，我能配置授权、健康、配额和失败处理，而看不到明文密钥。
- 作为阅读者，我能识别原始事实、派生内容和不可用来源。

## 功能要求

- FR-015-1：新部署不得依赖 Custom Search JSON API；存量适配器必须带明确 sunset 日期和迁移告警。
- FR-015-2：限定域场景优先评估 Vertex AI Search 等官方产品；全网搜索必须取得 Google 提供的正式方案与条款。
- FR-015-3：旧适配器只保存返回的标题、链接、摘要、时间和查询元数据，不抓取结果页。
- FR-015-4：能力档案记录 provider、contract_version 和 deprecation_at，到期自动阻止新 Monitor 发布。

## 非功能要求

- 外部请求必须具备固定端点、DNS/重定向复验、超时、限流和最大响应限制。
- 凭据只保存 `env:NAME` 引用；日志、错误、证据和前端不得暴露 Token。
- 采集、重试和人工触发保持幂等，可在 CollectionRun 中追踪。

## 非目标

不保证第三方未授权或已退役能力与参考项目保持表面一致。

## 验收标准

- AC-015-1：新客户配置流程不会引导创建已关闭 API
- AC-015-2：2027 停用日前有可观测迁移门禁
- AC-015-3：无替代授权时来源安全停用而非转网页抓取
