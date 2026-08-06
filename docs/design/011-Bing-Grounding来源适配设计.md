---
layer: Design
scope: backend
doc_no: "011"
title: Bing-Grounding来源适配设计
status: proposed
version: v1.0
owner: HotKey Team
phase: P2
canonical_path: docs/design/011-Bing-Grounding来源适配设计.md
prd: docs/prd/011-Bing-Grounding来源适配.md
plan: docs/plans/011-Bing-Grounding来源适配计划.md
---

# Bing-Grounding来源适配设计

## 现状

参考项目使用过 Bing 搜索，但 Microsoft Bing Search APIs 已于 2025-08-11 完全停用；当前可用方向是 Microsoft Foundry 的 Grounding/Web Search。

## 目标

在不伪装成原始搜索 API 的前提下，将 Bing Grounding 作为带引用的研究补充来源，为事件发现提供候选链接和摘要。

## 非目标

不使用未授权网页抓取、浏览器会话、验证码绕过或不稳定私有接口补齐能力；本条不实现其他编号的业务细节。

## 核心决策

- 禁止实现已退役 Bing Search API；适配器只接受当前官方 Foundry Web Search/Grounding 契约。
- Grounding 不向开发者返回原始搜索内容，因此输出标记为 derived_evidence，不能冒充抓取正文或原始指标。
- 完整保留 Microsoft 要求展示的引用链接和查询引用，不改变其格式；查询不得包含用户身份或敏感数据。
- 当业务必须获得原始文章时，仅对引用 URL 使用该站点自己的官方 API/RSS/授权 Feed 再采集。

## 数据、接口与交互

- Foundry 凭据与能力配置
- 带引用回答到候选证据的映射
- 查询、成本和数据边界审计
- 停用旧 Bing 选项和迁移提示

跨端契约先修改 Schema 与后端注解，再生成 OpenAPI 和前端 Client。来源能力必须在发布监控前可检测，禁用或阻塞状态不能进入调度。

## 安全、失败与降级

Grounding 的数据边界、展示义务和原始结果限制与普通连接器不同；未经法务和隐私评审不得默认启用。

第三方失败按认证、限流、临时、解析和永久错误分类；关闭单个来源不影响历史证据和其他来源。

## 设计验收边界

- 代码中不存在对已停用 Search API 的调用
- 所有 Bing 派生内容明确显示模型生成和引用
- 无 Foundry 配置时来源保持 disabled 而不影响其他连接器

## 官方依据

- https://learn.microsoft.com/en-us/lifecycle/announcements/bing-search-api-retirement
- https://learn.microsoft.com/en-us/azure/foundry/agents/how-to/tools/bing-tools
