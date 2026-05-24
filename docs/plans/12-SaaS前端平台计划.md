# 12 SaaS Frontend Platform Plan

## Summary

本阶段将现有 `hotkey-web` 从简易控制台升级为可私有部署、可直接使用的浅色企业 SaaS 前端平台。

本阶段只规划和实现前端产品体验，不改变后端数据模型，不新增后端认证、租户、计费或权限能力。后端 API 继续以 FastAPI 当前 OpenAPI 为准，报告能力统一使用 `/api/reports`，不恢复 `/api/daily-reports`。

## Current Status

- 已实现：官网首页、定价占位页、`/app` 工作台，以及热点、搜索、关键词、来源、任务、报告、通知、设置页面。
- 已实现：前端统一调用当前后端 API，报告使用 `/api/reports`，即时搜索使用 `/api/search`。
- 需后续验收：真实后端连接、真实凭据场景、桌面与移动端浏览器走查。

## Goals

- 使用 `Next.js 14 + TypeScript + Tailwind CSS + Radix UI primitives` 开发前端。
- 采用 shadcn 风格组件封装，形成轻量可复用的 UI 组件层。
- 提供 SaaS 产品形态：官网首页、定价占位页、应用工作台。
- 支持 AI 热点检测平台的主流程：热点、即时搜索、关键词、来源、任务、通知、日报/周报、设置。
- 以单用户私有部署为首版默认形态，确保用户可以直接使用。

## Non-goals

- 不做多用户账号体系。
- 不做租户隔离、组织、成员、邀请和角色权限。
- 不做真实登录认证。
- 不接 Stripe 或任何真实支付。
- 不新增后端聚合 API。
- 不恢复或兼容 `/api/daily-reports`。

## Product Shape

- `/`：SaaS 官网首页，介绍产品价值、核心能力、私有部署优势和工作台入口。
- `/pricing`：定价占位页，展示套餐结构和后续商业化方向，不接真实支付。
- `/app`：SaaS 工作台总览，展示关键指标、最近热点、任务状态、通知状态、报告入口和快速搜索。
- `/app/hotspots`：热点列表，支持筛选、排序和详情跳转。
- `/app/hotspots/[id]`：热点详情，展示来源、关键词、AI 分析、相关性理由和原始链接。
- `/app/search`：即时搜索，调用 `/api/search`，展示搜索结果、`active/filtered` 状态和来源错误。
- `/app/keywords`：关键词管理，支持新增、启停和删除。
- `/app/sources`：来源管理，支持 RSS、Hacker News、X/Twitter、Bing、Bilibili、Sogou-style。
- `/app/runs`：任务记录，支持手动触发热点检测。
- `/app/reports`：报告中心，支持 daily/weekly 报告生成、列表、预览和发送。
- `/app/reports/[id]`：报告详情，展示 Markdown 内容、周期、状态和发送信息。
- `/app/notifications`：通知记录，展示事件邮件和报告邮件状态。
- `/app/settings`：设置页，保留必要 JSON 设置编辑能力。

## Design Direction

- 风格：浅色企业 SaaS，专业、克制、高密度、可扫描。
- 色彩：以 `#F8FAFC` 为背景，navy/blue 为主色，amber/orange 作为 CTA 和重点提示。
- 字体：优先使用 `Plus Jakarta Sans`，通过 `next/font/google` 在 root layout 注入。
- 布局：工作台优先，不做营销式卡片堆叠；页面使用稳定侧边栏、顶部操作区和内容表格/列表。
- 图标：统一使用 `lucide-react`，不使用 emoji 作为 UI 图标。
- 交互：所有按钮、筛选、表单、菜单和对话框必须有清晰 hover、focus、loading 和 disabled 状态。
- 可访问性：正常文本对比度不低于 4.5:1；错误信息必须可被读屏识别；移动端触控目标不小于 44px。

## Component Strategy

- 安装必要 Radix primitives：
  - Dialog
  - Select
  - Tabs
  - DropdownMenu
  - Popover
  - Tooltip
  - Label
  - Switch
  - Checkbox
  - Separator
  - ScrollArea
  - Toast
- 安装辅助依赖：
  - `lucide-react`
  - `class-variance-authority`
  - `clsx`
  - `tailwind-merge`
- 建立 `src/components/ui/*`：
  - Button
  - Input
  - Textarea
  - Select
  - Dialog
  - Tabs
  - Badge
  - Card
  - Table
  - Tooltip
  - Switch
  - Toast
  - Skeleton

## Data and API Rules

- `NEXT_PUBLIC_API_BASE_URL` 继续作为 API base URL。
- 前端只消费当前后端 API，不在本阶段新增后端接口。
- 报告统一调用 `/api/reports`。
- 即时搜索调用 `/api/search`，搜索结果不入库、不影响报告。
- 手动检测调用 `/api/check-runs`。
- 热点、关键词、来源、任务、通知、设置继续使用现有 API。
- 前端代码、文案和测试不得出现 `/api/daily-reports`。

## UX Requirements

- 所有 mutation 必须有 loading 和 disabled 状态。
- 所有错误必须显示在当前上下文中，表单错误或请求错误使用 `role="alert"` 或 toast。
- 数据为空时必须显示空状态，不显示空白页面。
- 表格在桌面使用结构化 Table；移动端必须避免横向破版，可使用卡片或横向滚动容器。
- 响应式宽度至少覆盖 375px、768px、1024px、1440px。
- 文本不得溢出按钮、卡片和表格单元格。

## OpenSpec Execution

后续实现使用 OpenSpec change：`implement-saas-frontend-platform`。

执行顺序：

1. 创建并确认 OpenSpec artifacts。
2. 使用 `openspec-apply-change` 执行前端实现任务。
3. 每完成一个任务更新 `tasks.md` checkbox。
4. 使用 `openspec-verify-change` 验证实现与 artifacts 一致。
5. 运行 typecheck、build 和 agent-browser 验收。

## Verification

- 文档检查：
  - `docs/plans/12-SaaS前端平台计划.md` 存在。
  - `docs/product/执行计划导航.md` 引用本计划。
  - `AGENTS.md` 明确当前阶段允许实现 SaaS 前端。
  - 本计划明确不做多用户、租户、真实认证和真实计费。
  - 本计划明确不恢复 `/api/daily-reports`。
- 实现后检查：
  - `npm --prefix hotkey-web run typecheck`
  - `npm --prefix hotkey-web run build`
  - 使用 `agent-browser` 验证 `/`、`/pricing`、`/app`、`/app/search`、`/app/reports`。
  - 验证移动端无横向溢出，按钮和表单不重叠。
  - 验证页面不出现 `/api/daily-reports`。
