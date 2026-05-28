---
layer: Plan
doc_no: "32"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:frontend area:api"
purpose: "将 Web 前端从 mock 数据推进到接通真实后端 API，实现创作者工作台核心功能。"
canonical_path: "docs/plans/32-Web前端接真实API实现计划.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/26-系统端到端可运行与基础设施对接PRD.md
  - docs/superpowers/specs/2026-05-28-system-running-design.md
outputs:
  - Web 前端实现任务
  - Web 前端验证证据
triggers:
  - "docs/product/prd/26-系统端到端可运行与基础设施对接PRD.md 变更"
  - "后端 OpenAPI 规范变更"
  - "对应 GitHub issue 状态变更"
downstream:
  - docs/acceptance/
---

# 32-Web前端接真实API 实现计划

## 1. 目标

将 hotkey-web 从硬编码 mock 数据的 UI 原型推进到接通真实后端 API 的可用工作台，实现登录、热点榜单、热点详情、关键词管理、日报查看等核心功能。

## 2. 文件清单

- PRD：`docs/product/prd/26-系统端到端可运行与基础设施对接PRD.md`
- Plan：`docs/plans/32-Web前端接真实API实现计划.md`
- 后端 OpenAPI：`../hotkey-server/docs/openapi.json`
- 生成客户端目录：`src/services/hotkey/hotkey-server/`

## 3. 任务拆解

### 3.1 基础设施

**Task 32-1：引入第三方依赖**
- 安装：swr、zustand、react-hook-form、@hookform/resolvers、zod、axios、sonner、recharts
- 验收：`npm install` 成功，`tsc --noEmit` 无报错

**Task 32-2：重新生成 API 客户端**
- 确保后端 OpenAPI 规范已导出
- 运行 `npx openapi2ts` 重新生成
- 验收：`src/services/hotkey/hotkey-server/` 包含最新类型和函数

**Task 32-3：请求层重构**
- `src/lib/request.ts`：基于 axios 封装，自动注入 token，401 拦截
- 适配 `@umijs/openapi` 生成代码使用 axios 实例
- 验收：请求工具可正常发起带 token 的请求

### 3.2 认证体系

**Task 32-4：zustand 认证 store**
- `src/stores/auth.ts`：token、user、login/logout actions
- 持久化到 localStorage
- 验收：登录后 token 存储，刷新页面后登录态保持

**Task 32-5：useAuth hook**
- `src/hooks/useAuth.ts`：封装 auth store
- 提供 isAuthenticated、user、login、logout
- 验收：组件可正常使用 useAuth

**Task 32-6：登录页**
- `app/(auth)/login/page.tsx`
- 使用 react-hook-form + zod 校验
- 调用 `postApiV1AuthEmailLogin`
- 验收：输入邮箱密码 → 登录成功 → 跳转工作台

### 3.3 页面路由

**Task 32-7：工作台布局**
- `app/(dashboard)/layout.tsx`：侧边栏 + 顶部导航
- 使用 `/frontend-design` 技能设计
- 验收：布局美观，导航可点击

**Task 32-8：热点榜单页**
- `app/(dashboard)/page.tsx`
- 使用 swr 调用 `getApiV1Hotspots`
- 使用 recharts 展示趋势图
- 使用 `/frontend-design` 技能设计
- 验收：展示真实热点数据，支持筛选排序

**Task 32-9：热点详情页**
- `app/(dashboard)/hotspots/[id]/page.tsx`
- 调用 `getApiV1HotspotsId`
- 展示证据链、AI 摘要
- 使用 `/frontend-design` 技能设计
- 验收：详情页展示完整信息

**Task 32-10：关键词管理页**
- `app/(dashboard)/keywords/page.tsx`
- 调用关键词相关 API
- 支持关注/屏蔽/添加
- 使用 `/frontend-design` 技能设计
- 验收：关键词 CRUD 正常，刷新后保持

**Task 32-11：日报页**
- `app/(dashboard)/reports/page.tsx`
- 调用 `getApiV1ReportsDaily`
- 使用 `/frontend-design` 技能设计
- 验收：日报列表和内容展示正常

**Task 32-12：设置页**
- `app/(dashboard)/settings/page.tsx`
- 来源配置展示
- 使用 `/frontend-design` 技能设计
- 验收：来源列表展示正常

### 3.4 组件拆分

**Task 32-13：拆分 CreatorWorkbench**
- 从巨型组件拆分为独立组件：
  - `src/components/layout/Sidebar.tsx`
  - `src/components/layout/Header.tsx`
  - `src/components/hotspot/HotspotList.tsx`
  - `src/components/hotspot/HotspotCard.tsx`
  - `src/components/hotspot/HotspotDetail.tsx`
  - `src/components/keyword/KeywordManager.tsx`
  - `src/components/report/DailyReport.tsx`
- 验收：各组件独立可渲染，原页面功能不退化

**Task 32-14：新增 shadcn/ui 组件**
- 通过 `npx shadcn@latest add` 添加：input、dialog、badge、skeleton、toast
- 验收：组件可正常使用

### 3.5 数据 hooks

**Task 32-15：数据获取 hooks**
- `src/hooks/useHotspots.ts`：swr + getApiV1Hotspots
- `src/hooks/useHotspotDetail.ts`：swr + getApiV1HotspotsId
- `src/hooks/useKeywords.ts`：关键词相关 API
- `src/hooks/useReports.ts`：日报 API
- 验收：hooks 返回 loading/error/data 状态正确

## 4. TDD 与验证

- 类型检查：每个 Task 完成后 `tsc --noEmit`
- 构建检查：`npm run build`
- 手动验证：登录 → 热点列表 → 热点详情 → 关键词管理完整链路
- 组件测试：核心组件使用 vitest + @testing-library/react（可选，不强制）

## 5. 执行顺序

```
32-1 → 32-2 → 32-3（基础设施）
32-4 → 32-5 → 32-6（认证体系）
32-7 → 32-13 → 32-14（布局与组件拆分）
32-8 → 32-9 → 32-10 → 32-11 → 32-12（各页面，可并行）
32-15（数据 hooks，与页面同步）
```

## 6. 回滚策略

- 第三方依赖：可随时卸载，不影响现有代码
- 路由变更：保留原有 page.tsx 作为 fallback
- 组件拆分：逐步迁移，旧组件保留直到新组件验证通过

## 7. 验收标准

- `npm run dev` 启动成功
- 登录 → 热点榜单 → 热点详情 → 关键词管理 → 日报链路通
- 所有页面展示真实后端数据
- `tsc --noEmit` 无报错
- `npm run build` 构建成功
- UI 美观灵动（通过 `/frontend-design` 保障）

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-28 | StephenQiu30 | 1.0.0 | 初版，Web 前端接真实 API 实现计划 |
