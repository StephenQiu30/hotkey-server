---
layer: Design
scope: backend
doc_no: "011"
title: Bing-Grounding来源适配设计
status: accepted
version: v1.1
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

- 来源类型固定为 `bing_grounding`，端点只接受
  `https://<account>.services.ai.azure.com/api/projects/<project>/toolboxes/<name>/versions/<version>/mcp?api-version=v1`。
- 身份只保存 `env:NAME` 引用，运行时读取面向 `https://ai.azure.com/.default` 的短期 Entra Bearer Token。
- MCP 客户端按 `initialize → notifications/initialized → tools/list → tools/call` 执行；所有请求携带
  `Foundry-Features: Toolboxes=V1Preview`，搜索调用使用服务端返回的唯一 Web Search 工具名和
  `{"search_query":"..."}`，并请求 `text/event-stream`。
- 健康探测只完成初始化和工具清单校验，不触发计费搜索；采集调用一次搜索且不虚构分页。
- `resource.text` 原样作为派生摘要保存，`resource._meta.annotations` 中的 URL 引用映射为附件，首个引用作为候选 URL；标题、作者和 `summary_only` 明确标记模型派生证据，不生成来源指标。
- 查询沿用 CollectionRun 的 query signature，并拒绝控制字符、电子邮箱、密钥/令牌形态等敏感输入。
- `grounding_data_boundary_approved` 是显式合规门禁；未确认 Microsoft DPA 不适用、数据可能越出地理和合规边界、额外条款与成本前，不允许探测、采集或启用。
- 来源创建后固定 disabled；只有合规门禁通过且健康状态为 healthy 才能启用。

跨端契约先修改 Schema 与后端注解，再生成 OpenAPI 和前端 Client。来源能力必须在发布监控前可检测，禁用或阻塞状态不能进入调度。

## 安全、失败与降级

Grounding 的数据边界、展示义务和原始结果限制与普通连接器不同；未经法务和隐私评审不得启用。

端点执行静态校验以及运行时 DNS、每次重定向、公网 IP、同主机同路径复验；禁用系统代理，限制 TLS 1.2、超时和 4 MiB 响应。401/403、429、5xx、MCP 错误、SSE/JSON 解析错误和超限响应均转为稳定、安全且不包含 Token/响应正文的诊断。

第三方失败按认证、限流、临时、解析和永久错误分类；关闭单个来源不影响历史证据和其他来源。

## 设计验收边界

- 代码中不存在对已停用 Search API 的调用
- 所有 Bing 派生内容明确显示模型生成和引用
- 无 Foundry 配置时来源保持 disabled 而不影响其他连接器
- 健康探测不产生搜索费用；未确认数据边界或 Toolbox 契约不符时不能启用
- 引用 URL、查询签名和派生摘要可审计，响应正文与 Token 不进入错误或日志

## 官方依据

- https://learn.microsoft.com/en-us/lifecycle/announcements/bing-search-api-retirement
- https://learn.microsoft.com/en-us/azure/ai-foundry/agents/how-to/tools/web-search?view=foundry
- https://learn.microsoft.com/en-us/azure/foundry/agents/how-to/tools/toolbox
