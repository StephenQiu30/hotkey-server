---
layer: Plan
scope: fullstack
doc_no: "016"
title: DuckDuckGo-Instant-Answer边界计划
status: completed
version: v1.1
owner: HotKey Team
phase: P2
canonical_path: docs/plans/archive/016-DuckDuckGo-Instant-Answer边界计划.md
design: docs/design/archive/016-DuckDuckGo-Instant-Answer边界设计.md
prd: docs/prd/archive/016-DuckDuckGo-Instant-Answer边界.md
---

# DuckDuckGo Instant Answer 边界计划

## 前置门禁

- [x] Design 已 accepted，PRD 已 approved。
- [x] 官方 Instant Answers、结果来源、搜索结果和服务条款已复核。
- [x] 确认官方当前没有满足生产接入要求的第三方 Instant Answer API 契约，因此不得创建 Connector。
- [x] 确认仓库没有既有 DuckDuckGo SourceType、Schema 数据、连接、内容或调度需要迁移。

## 执行步骤

1. [x] 先增加架构失败测试，禁止 DuckDuckGo Connector、HTML/Lite 搜索页、历史 API 地址和 Schema 枚举。
2. [x] 新增复用 shadcn/ui 的只读能力卡，展示“未开放”、事实边界、官方链接和解锁条件。
3. [x] 将能力卡接入来源页，确保管理员与 Viewer 可见，来源创建器没有 DuckDuckGo 选项。
4. [x] 增加界面测试，覆盖官方链接、不可执行按钮、无创建选项和无 API 请求。
5. [x] 运行后端架构/仓库测试、前端全量回归、生产构建、Compose 检查和 `git diff --check`。
6. [x] 使用 agent-browser 验收管理员、Viewer、匿名、390px 移动端、官方链接语义和 axe。
7. [x] 新建同编号 Acceptance，完成后按中文 Conventional Commits 独立提交。

## 验证命令

- `cd backend && go test ./test/architecture -run DuckDuckGo -count=1 && make validate`
- `cd frontend && npm run openapi:check && npm run typecheck && npm run test:unit && npm run build`
- 根目录运行 Compose 开发/生产配置检查与 `git diff --check`。

## 测试策略

- 架构测试直接扫描生产 Go、Connector 目录和唯一 Schema，不依赖字符串约定或人工审查。
- React 测试 mock 既有来源 API，证明渲染能力卡不会新增 DuckDuckGo 请求。
- 浏览器只验证 HotKey 本地页面；不点击外链、不向 DuckDuckGo 发起验收请求。

## 回滚

回滚只移除 DuckDuckGo 边界卡、接入点和对应门禁测试；不涉及数据库、来源、调度、内容、指标或凭据。

## 完成定义

- AC-016-1..5 全部绑定自动化或浏览器证据。
- 没有 DuckDuckGo Connector、SourceType、Schema 枚举、外部请求或热度旁路。
- 没有真实凭据、临时验收文件或生成漂移进入提交。
- 工作区只包含第 016 项变更并完成独立中文提交。
