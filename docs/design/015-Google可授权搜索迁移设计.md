---
layer: Design
scope: fullstack
doc_no: "015"
title: Google可授权搜索迁移设计
status: accepted
version: v1.1
owner: HotKey Team
phase: P2
canonical_path: docs/design/015-Google可授权搜索迁移设计.md
prd: docs/prd/015-Google可授权搜索迁移.md
plan: docs/plans/015-Google可授权搜索迁移计划.md
---

# Google 可授权搜索迁移设计

## 现状与官方边界

Google 官方文档在 2026-02-18 更新后明确：Custom Search JSON API 已停止接纳新客户，存量客户必须在 2027-01-01 前迁移；最多 50 个域的场景推荐使用 Agent Search（原 Vertex AI Search），全网搜索需联系 Google 取得正式方案。Agent Search 使用 Discovery Engine v1 REST API，只搜索客户已配置的数据存储，不等价于开放全网搜索。

## 目标

为新部署提供 Agent Search 限定域关键词来源；同时在产品内明确旧 API 停用日期和全网搜索授权门禁，确保系统不会回退到 Google 搜索页抓取。

## MVP 决策

- 新增来源类型 `google_agent_search`，只调用 Discovery Engine v1 `servingConfigs.search`。
- 不新增 Custom Search JSON API 连接器或配置入口；本仓库没有需要兼容的既有 Google 连接记录。
- 连接配置固定为 `global`、`us` 或 `eu` 官方端点，保存完整 ServingConfig 资源名；MVP 仅支持 `dataStores` 路径，不接受 `engines`、任意 URL 或请求级端点。
- 凭据只保存 `env:NAME` Bearer 访问令牌引用。生产环境应使用 ADC/工作负载身份产生短期令牌；本条不保存服务账号 JSON。
- 创建后默认禁用；健康探测通过后才能启用。探测读取固定 ServingConfig，采集仅提交查询、页大小、页令牌、SafeSearch 与 snippet 请求。
- 能力档案固定记录 provider=`google-agent-search`、contract_version=`discoveryengine-v1`、legacy_deprecation_at=`2027-01-01`，用于界面提示与审计；新来源本身不设停用日。
- 仅保存 Google 返回的文档 ID、标题、链接、snippet 和返回时间；不抓取结果链接，不把 snippet 标记为网页正文。

## 数据与契约

`SourceConfig` 增加 Google location 和 ServingConfig 资源名；Schema、HTTP DTO、OpenAPI 与前端 Client 同步生成。连接器把官方 `nextPageToken` 封装为与查询签名绑定的检查点，最多按来源 `max_pages_per_run` 调度。

结果映射为既有 `SourceItem`：文档 ID 为外部 ID，标题与 `derivedStructData.link` 为事实定位，首个成功 snippet 为摘要，采集时间为观察时间。缺少安全 HTTPS 链接或稳定 ID 的结果只产出诊断，不进入证据流水线。

## 安全与失败

- 仅允许 `discoveryengine.googleapis.com`、`us-discoveryengine.googleapis.com`、`eu-discoveryengine.googleapis.com`，TLS 1.2+，DNS、连接与每次重定向复验，4 MiB 响应上限。
- location、端点和资源名必须一致；资源名只允许安全字符和 `default_collection`。
- 401/403 归类认证失败，429 归类限流，5xx 归类临时失败，4xx 合同/配置错误归类永久失败；第三方正文不得进入错误信息。
- 旧 Custom Search endpoint、Google 网页/移动页、验证码绕过和私有接口由架构测试禁止。

## 验收边界

- AC-015-1：新建来源只出现 Agent Search，不出现 Custom Search JSON API，并展示 2027-01-01 停用告警。
- AC-015-2：官方端点、location、ServingConfig、Bearer env 引用和健康状态均被严格校验；未健康来源不能启用。
- AC-015-3：关键词搜索、分页、结果映射、认证、限流、临时失败、恶意 DNS/重定向均有自动化测试。
- AC-015-4：无正式全网搜索授权时系统安全停用，不请求 Google 搜索网页或非官方接口。
- AC-015-5：管理员可配置和探测；阅读者只能查看脱敏状态；桌面与移动关键流程无阻塞可访问性问题。

## 官方依据

- https://developers.google.com/custom-search/v1/overview
- https://docs.cloud.google.com/generative-ai-app-builder/docs
- https://docs.cloud.google.com/generative-ai-app-builder/docs/locations
- https://docs.cloud.google.com/generative-ai-app-builder/docs/authentication
- https://docs.cloud.google.com/generative-ai-app-builder/docs/reference/rest/v1/projects.locations.collections.dataStores.servingConfigs/search
- https://docs.cloud.google.com/generative-ai-app-builder/docs/snippets
