---
layer: Acceptance
scope: shared
doc_no: "016"
title: DuckDuckGo-Instant-Answer边界验收
status: passed
version: v1.0
owner: HotKey Team
phase: P2
canonical_path: docs/acceptance/016-DuckDuckGo-Instant-Answer边界验收.md
design: docs/design/archive/016-DuckDuckGo-Instant-Answer边界设计.md
prd: docs/prd/archive/016-DuckDuckGo-Instant-Answer边界.md
plan: docs/plans/archive/016-DuckDuckGo-Instant-Answer边界计划.md
---

# DuckDuckGo Instant Answer 边界验收

## 验收结论

016 于 2026-08-07 通过验收。HotKey 将 DuckDuckGo Instant Answer 明确建模为“未开放”的产品能力边界：界面说明它是知识答案而非通用网页搜索结果 API，并提供当前官方说明；生产代码没有 DuckDuckGo Connector、SourceType、Schema 枚举、外部请求、内容、调度、指标或热度旁路。

## 验收标准

| 标准 | 结果 | 证据 |
|---|---|---|
| AC-016-1 | passed | 管理员与 Viewer 均看到“DuckDuckGo Instant Answer”“未开放”、能力边界、三条官方链接、四项未来解锁条件和禁用状态按钮。 |
| AC-016-2 | passed | 来源选择器实际展开后只包含既有七类正式来源，没有 DuckDuckGo；唯一 Schema 和 Source Domain 没有 DuckDuckGo 枚举，因此无法创建或调度。 |
| AC-016-3 | passed | 架构测试会在 DuckDuckGo Connector 目录、HTML/Lite 搜索页、历史 Instant Answer API 地址或 Schema 枚举出现时失败。 |
| AC-016-4 | passed | 后端全量 CI、前端 143 项测试、生产构建、Compose、管理员/Viewer/匿名、390px 和 axe 全部通过。 |
| AC-016-5 | passed | 浏览器网络日志、生产 Go 扫描和仓库检索均为 0 个 DuckDuckGo 请求；没有真实凭据或临时验收资产进入提交。 |

## 自动化验证

```bash
cd backend
HOTKEY_TEST_DSN='postgres://hotkey:hotkey@127.0.0.1:55436/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:56383/15' make ci

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build

cd ..
docker compose -f docker-compose.yml config --quiet
docker compose --env-file .env.prod -f docker-compose-prod.yml config --quiet
git diff --check
```

后端全量 CI 使用 `pgvector/pgvector:pg18` 和 Redis 8，通过 OpenAPI 再生成一致性、vet、PostgreSQL 18 运行时与容量门禁、数据库升降级、68 表 Schema 指纹、全包测试、构建、架构和仓库门禁。前端 OpenAPI 契约、类型检查、37 个测试文件共 143 项测试及 Next.js 生产构建通过。Compose 开发/生产配置通过；生产检查只注入本地占位变量，不启动或修改生产服务。

React 最佳实践复核通过：新卡片是无状态静态组件，没有 Hook、Effect、客户端请求、派生状态或新依赖；全部交互复用 shadcn/ui Card、Badge 和 Button，外链使用明确名称与安全的新窗口属性。

## 浏览器验收

使用用户指定的 `$agent-browser` 连接真实 Next.js、Go、PostgreSQL、Redis 与 MinIO：

- 管理员来源页显示 DuckDuckGo 边界卡、完整说明和禁用按钮；三条官方链接 href 正确，验收未点击外链。
- 管理员展开来源类型 Select，只有 RSS / Atom、Hacker News、X、Microsoft Foundry Web Search、Bilibili、微博和 Google Agent Search，没有 DuckDuckGo。
- Viewer 来源目录显示相同事实边界，不出现新增来源按钮。
- 未认证访问 `/dashboard/sources` 跳转到 `/login?redirect=%2Fdashboard%2Fsources`。
- 390×844 下文档宽度为 390px、卡片宽度为 358px，无横向溢出。管理员与 Viewer 页面 axe-core 4.12.1 均为 0 violations、0 incomplete。
- 网络记录按 `duckduckgo` 过滤为 0 条；页面错误为空，控制台只有 React 开发提示和 HMR 连接信息。

截图和快照保存在本地 `/tmp/hotkey-016-dogfood/`，不提交为仓库事实源。

## 官方依据与能力边界

- [DuckDuckGo Instant Answers](https://duckduckgo.com/duckduckgo-help-pages/features/instant-answers-and-other-features)把 Instant Answers 描述为搜索结果中的即时知识答案，并说明其来源广泛。
- [DuckDuckGo 结果来源](https://duckduckgo.com/duckduckgo-help-pages/results/sources)区分 Instant Answers 与传统链接，并说明传统链接大量来自 Bing。
- [DuckDuckGo 搜索结果说明](https://duckduckgo.com/duckduckgo-help-pages/results)描述面向终端用户的搜索产品功能，没有给出第三方生产 API 契约。
- [DuckDuckGo 服务条款](https://duckduckgo.com/terms)要求授权使用和遵守 Acceptable Use Policy，并明确不保证 Instant Answers 的准确性或完整性。
- 2026-08-07 复核时，历史 `https://duckduckgo.com/api` 文档入口已重定向到搜索页面；即使历史 JSON 端点仍可能响应，也不满足本项目对版本、配额、SLA、存储、归属和删除同步的生产要求，因此不会探测或接入。

## 回滚

回滚只移除 DuckDuckGo 边界卡、来源页接入点和架构门禁测试。没有 Schema、来源、调度、内容、指标、检查点或凭据需要回滚，既有来源行为不变。
