# Proposal: 用户前台与管理后台

## What

创建 Next.js Web 工程，提供用户前台（登录、监控任务、内容流、主题、趋势、提醒）和管理后台（任务运行、连接器状态）。

## Why

创作者需要可视化界面来查看监控任务结果、热点内容、主题趋势和通知提醒，管理后台用于运维监控。

## Normative files changed

- `web/` (new directory, full web application)
- `docs/plans/006-用户前台与后台计划.md` (existing plan, referenced as source of truth)

## Non-goals

- 不做复杂品牌视觉系统
- 不做小程序端
- 不修改后端 API

## Scope

1. Next.js 工程初始化（TypeScript, Vitest, ESLint）
2. API 客户端封装
3. 用户登录页
4. 监控任务列表页
5. 热点内容、主题和趋势页面
6. 提醒中心页面
7. 管理后台（任务运行、连接器状态）
