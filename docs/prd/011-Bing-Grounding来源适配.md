---
layer: PRD
scope: backend
doc_no: "011"
title: Bing-Grounding来源适配
status: draft
version: v1.0
owner: HotKey Team
phase: P2
canonical_path: docs/prd/011-Bing-Grounding来源适配.md
design: docs/design/011-Bing-Grounding来源适配设计.md
plan: docs/plans/011-Bing-Grounding来源适配计划.md
---

# Bing-Grounding来源适配

## 用户问题

参考项目使用过 Bing 搜索，但 Microsoft Bing Search APIs 已于 2025-08-11 完全停用；当前可用方向是 Microsoft Foundry 的 Grounding/Web Search。

## 产品目标

在不伪装成原始搜索 API 的前提下，将 Bing Grounding 作为带引用的研究补充来源，为事件发现提供候选链接和摘要。

## 范围

- Foundry 凭据与能力配置
- 带引用回答到候选证据的映射
- 查询、成本和数据边界审计
- 停用旧 Bing 选项和迁移提示

## 用户故事

- 作为编辑者，我能选择真实可用的来源能力并预览查询结果。
- 作为管理员，我能配置授权、健康、配额和失败处理，而看不到明文密钥。
- 作为阅读者，我能识别原始事实、派生内容和不可用来源。

## 功能要求

- FR-011-1：禁止实现已退役 Bing Search API；适配器只接受当前官方 Foundry Web Search/Grounding 契约。
- FR-011-2：Grounding 不向开发者返回原始搜索内容，因此输出标记为 derived_evidence，不能冒充抓取正文或原始指标。
- FR-011-3：完整保留 Microsoft 要求展示的引用链接和查询引用，不改变其格式；查询不得包含用户身份或敏感数据。
- FR-011-4：当业务必须获得原始文章时，仅对引用 URL 使用该站点自己的官方 API/RSS/授权 Feed 再采集。

## 非功能要求

- 外部请求必须具备固定端点、DNS/重定向复验、超时、限流和最大响应限制。
- 凭据只保存 `env:NAME` 引用；日志、错误、证据和前端不得暴露 Token。
- 采集、重试和人工触发保持幂等，可在 CollectionRun 中追踪。

## 非目标

不保证第三方未授权或已退役能力与参考项目保持表面一致。

## 验收标准

- AC-011-1：代码中不存在对已停用 Search API 的调用
- AC-011-2：所有 Bing 派生内容明确显示模型生成和引用
- AC-011-3：无 Foundry 配置时来源保持 disabled 而不影响其他连接器
