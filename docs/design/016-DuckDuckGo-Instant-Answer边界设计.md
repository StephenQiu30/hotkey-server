---
layer: Design
scope: fullstack
doc_no: "016"
title: DuckDuckGo-Instant-Answer边界设计
status: accepted
version: v1.1
owner: HotKey Team
phase: P2
canonical_path: docs/design/016-DuckDuckGo-Instant-Answer边界设计.md
prd: docs/prd/016-DuckDuckGo-Instant-Answer边界.md
plan: docs/plans/016-DuckDuckGo-Instant-Answer边界计划.md
---

# DuckDuckGo Instant Answer 边界设计

## 背景与事实

参考实现通过非官方方式取得 DuckDuckGo 结果，但 HotKey 需要可持续、可审计的生产来源。2026-08-07 复核 DuckDuckGo 官方资料后确认：

- 官方将 Instant Answers 描述为搜索结果页顶部的即时答案功能，内容来自 DuckDuckGo 自有索引和一百多个第三方来源。
- 官方来源说明把 Instant Answers、传统链接和广告区分为不同结果类型；传统链接大量来自 Bing，因此 Instant Answer 不能代表有机网页搜索结果。
- 当前官方帮助中心没有发布供第三方生产系统使用的 Instant Answer 服务端 API 契约、版本、配额、SLA、分页或删除同步规则；历史 `/api` 文档入口当前重定向到搜索页面。
- DuckDuckGo 服务条款要求按授权方式使用服务并遵守 Acceptable Use Policy，也明确不保证 Instant Answers 的准确性或完整性。

因此，本条不是“少做一个搜索连接器”，而是明确防止把未文档化端点或网页抓取伪装为正式数据源。

## 目标

- 在来源管理页公开说明 DuckDuckGo 当前不可作为 HotKey 的全网搜索或定时监听来源。
- 通过代码和 Schema 门禁阻止非官方网页、历史未文档化端点或伪造来源类型进入生产。
- 给出未来正式合作接口的最小解锁条件，避免后续实现者误读边界。
- 不影响 RSS、Hacker News、X、Bilibili、微博、Google Agent Search 和 Microsoft Foundry Web Search。

## 非目标

- 不调用 `api.duckduckgo.com` 历史 Instant Answer JSON 端点。
- 不抓取 DuckDuckGo HTML、Lite、非 JavaScript 搜索页、自动完成、图片、新闻或私有接口。
- 不创建 `duckduckgo` / `duckduckgo_instant_answer` SourceType、Connector、指标能力、检查点或调度任务。
- 不把 `!bang`、Search Assist、Duck.ai 或第三方 DuckDuckGo SDK 纳入 MVP。
- 不用 SerpAPI、SearXNG 等第三方代理冒充 DuckDuckGo 官方来源。

## 核心设计

### 1. 产品状态

来源管理页展示只读的“DuckDuckGo Instant Answer”能力卡：

- 状态固定为“未开放”；
- 说明 Instant Answer 是知识答案，不是网页搜索结果 API；
- 明确 HotKey 不抓取页面、不创建来源、不进入调度或热度计算；
- 链接到 DuckDuckGo 官方 Instant Answers、结果来源和服务条款；
- 管理员与 Viewer 看到同一事实边界，不提供配置或启用操作。

该卡使用现有 shadcn/ui Card、Badge、Button，不创建新的个性化交互模式。

### 2. 生产门禁

架构测试必须同时保证：

- `internal/modules/source/infrastructure/duckduckgo` 不存在；
- 生产 Go 代码不包含 DuckDuckGo 搜索页、Lite/HTML 或历史 Instant Answer API 地址；
- `db/schema.sql` 不注册 DuckDuckGo 来源类型；
- 前端来源选择器没有 DuckDuckGo 可执行选项。

这些门禁是可执行的产品约束，而不是仅靠文档提醒。

### 3. 数据和热度边界

由于 MVP 不创建 DuckDuckGo Content，现有标准化、相关性、事件聚类、热度和指标流水线无需特殊分支。不存在“knowledge_answer 先落库再排除热度”的旁路，避免引入没有业务收益的模型与迁移。

未来若取得正式接口，必须重新设计独立的 `knowledge_answer` 证据类型，并默认满足：

- 只作为知识补充，不计来源传播宽度或平台热度；
- 保存上游来源归属、更新时间、删除/撤回状态与完整证据链接；
- 与通用网页搜索、新闻搜索和社交指标能力严格分离。

### 4. 解锁条件

只有同时取得以下材料，才能新开编号任务评估 Connector：

1. DuckDuckGo 发布或书面授予的服务端读取权；
2. 固定 HTTPS 端点、认证、版本、请求/响应和速率配额；
3. 商用存储、展示归属、第三方内容权利和删除同步条款；
4. 可控沙盒或官方 fixture；
5. 失败分类、SSRF、重定向、响应上限和撤权行为可验证。

## 安全与失败

- 不存在外部请求，因此不新增凭据、Token、DNS、重定向、限流或响应解析攻击面。
- 禁用卡不依赖 DuckDuckGo 在线状态；官方链接失效不会把能力误变为可用。
- 未来正式能力不得用浏览器 Cookie、验证码绕过、代理池或不稳定私有接口降级。
- 历史已有来源不会因本条受影响，回滚只需移除说明卡与架构门禁。

## 验收边界

- UI 明确显示“未开放”，且不存在 DuckDuckGo 来源配置选项。
- 生产代码和 Schema 不含可执行 DuckDuckGo 来源能力。
- 自动化测试证明 DuckDuckGo 不创建来源、调度、内容或指标。
- 桌面、移动端、Viewer 和匿名权限链路通过浏览器验收。

## 官方依据

- [DuckDuckGo Instant Answers](https://duckduckgo.com/duckduckgo-help-pages/features/instant-answers-and-other-features)
- [DuckDuckGo 结果来源](https://duckduckgo.com/duckduckgo-help-pages/results/sources)
- [DuckDuckGo 搜索结果说明](https://duckduckgo.com/duckduckgo-help-pages/results)
- [DuckDuckGo 服务条款](https://duckduckgo.com/terms)
