---
layer: PRD
scope: fullstack
doc_no: "016"
title: DuckDuckGo-Instant-Answer边界
status: approved
version: v1.1
owner: HotKey Team
phase: P2
canonical_path: docs/prd/016-DuckDuckGo-Instant-Answer边界.md
design: docs/design/016-DuckDuckGo-Instant-Answer边界设计.md
plan: docs/plans/016-DuckDuckGo-Instant-Answer边界计划.md
---

# DuckDuckGo Instant Answer 边界

## 用户问题

用户可能因参考项目或历史 `/api` 示例而认为 DuckDuckGo 提供免费全网搜索 API。当前官方资料只把 Instant Answers 定义为搜索产品中的知识答案功能，没有发布满足 HotKey 生产接入要求的第三方服务端 API 契约。若界面不说明边界，管理员可能误以为缺少配置，开发者也可能以网页抓取或未文档化端点补齐。

## 产品目标

在不引入无授权外部调用的前提下，让管理员和 Viewer 清楚识别 DuckDuckGo 的当前能力状态，并用自动化门禁保证它不会进入来源、调度、内容或热度链路。

## 用户故事

- 作为管理员，我能看到 DuckDuckGo 为什么未开放、官方依据和未来解锁条件，不会误建不可用来源。
- 作为 Viewer，我能区分“产品未开放”与“来源临时故障”，且看不到虚假的可配置操作。
- 作为开发者，我会被测试阻止引入 DuckDuckGo 网页抓取、历史未文档化 API 或 Schema 来源枚举。

## 功能要求

- FR-016-1：来源管理页展示“DuckDuckGo Instant Answer”只读能力卡，状态固定为“未开放”。
- FR-016-2：卡片必须说明 Instant Answer 是知识答案而非通用网页搜索结果 API；HotKey 不抓取页面、不创建来源、不进入调度和热度计算。
- FR-016-3：卡片提供 DuckDuckGo 官方 Instant Answers、结果来源和服务条款链接；禁用按钮不得触发请求或导航。
- FR-016-4：来源创建器不出现 DuckDuckGo 选项，后端不注册 SourceType、Connector、指标能力或数据库枚举。
- FR-016-5：架构测试禁止 DuckDuckGo HTML/Lite 搜索页、历史 Instant Answer API 地址和对应 Connector 目录进入生产代码。
- FR-016-6：未来解锁说明至少覆盖正式读取权、端点/认证/版本/配额、存储与归属条款、删除同步和官方 fixture。

## 非功能要求

- 使用现有 shadcn/ui Card、Badge、Button 和响应式布局，不新增页面级状态或外部请求。
- 390px 宽度下无横向溢出；键盘焦点、链接名称和标题层级可访问。
- 管理员与 Viewer 看到同一事实边界；匿名用户仍由既有 AuthGuard 重定向。
- 自动化测试不得访问 DuckDuckGo 生产服务，也不得依赖网络状态。

## 非目标

- 不实现 Instant Answer、网页、新闻、图片、自动完成、Duck.ai、Search Assist 或 `!bang` Connector。
- 不创建 `knowledge_answer` 数据、热度排除分支、检查点或迁移。
- 不评估或接入第三方 DuckDuckGo 抓取 SDK、代理搜索服务或浏览器自动化采集。

## 验收标准

- AC-016-1：管理员与 Viewer 均能看到“未开放”卡、能力边界、三条官方链接和不可执行状态按钮。
- AC-016-2：来源选择器没有 DuckDuckGo；Schema 和 Source Domain 不存在 DuckDuckGo 来源枚举，因而无法创建或调度。
- AC-016-3：架构测试会在生产 Go 代码出现 DuckDuckGo HTML/Lite、历史 API 地址或 Connector 目录时失败。
- AC-016-4：现有来源全量回归、前端测试、生产构建、桌面/移动、权限和 axe 验收通过。
- AC-016-5：仓库没有 DuckDuckGo Content、指标或热度旁路，也没有真实请求、凭据和临时验收资产进入提交。

## 成功指标

- 0 个可执行 DuckDuckGo 来源入口。
- 0 个 DuckDuckGo 外部请求和生产端点常量。
- 100% AC 绑定自动化或浏览器证据。

## 发布与回滚

该能力卡随普通前端版本发布，无开关、Schema 迁移或外部依赖。回滚仅移除卡片和架构门禁，不影响任何已有来源、内容、运行或指标。
