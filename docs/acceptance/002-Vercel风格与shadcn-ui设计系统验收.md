---
layer: Acceptance
scope: frontend
doc_no: "002"
title: Vercel风格与shadcn-ui设计系统验收
status: passed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/acceptance/002-Vercel风格与shadcn-ui设计系统验收.md
design: docs/design/002-Vercel风格与shadcn-ui设计系统设计.md
prd: docs/prd/002-Vercel风格与shadcn-ui设计系统.md
plan: docs/plans/002-Vercel风格与shadcn-ui设计系统计划.md
---

# Vercel风格与shadcn-ui设计系统验收

## 验收结论

002 通过验收。前端继续使用 Next.js App Router、TypeScript、Tailwind CSS v4、Radix 与 shadcn/ui 官方组合；页面以 Vercel 首页的信息层级、留白、细边界和中性黑白层级为参考，不复制商标、文案或受保护资产。亮暗主题、响应式导航、统一状态组件与可访问性形成可运行闭环。

## 设计审计

- 审计范围：公开首页、登录页、工作台与全部顶层工作台页面，覆盖 1440 × 1000 桌面和 390 × 844 移动视口。
- 用户目标：快速理解产品定位，在任何主题和视口下完成导航、检索、认证与空态恢复操作。
- 已确认优势：现有页面已复用 shadcn/ui 的 Button、Input、NavigationMenu、Sheet、Dialog、Table、Alert、Skeleton、Empty 与 Sonner；页面容器和状态语言一致。
- 已修复风险：原 ThemeProvider 固定亮色、ThemeToggle 未挂载、暗色令牌缺失、图表与内容阅读面存在硬编码亮色、Tailwind v4 的 shadcn 配置仍指向旧配置文件。
- 证据边界：Vercel 仅作为视觉层级参考；业务结构、中文内容、品牌资产和交互语义均保持 HotKey 自有实现。

参考依据为 [Vercel 首页](https://vercel.com/home)、[shadcn/ui 组件原则](https://ui.shadcn.com/docs)、[shadcn/ui 主题规范](https://ui.shadcn.com/docs/theming) 和 [Tailwind v4 配置说明](https://ui.shadcn.com/docs/tailwind-v4)。`npx shadcn@latest info` 已确认当前项目为 Next.js 16.3、React 19、Tailwind v4、new-york、Radix 与 CSS Variables 配置。

## 验收标准

| 标准 | 结果 | 证据 |
|---|---|---|
| AC-002-1 | passed | 源码扫描仅保留 Markdown 渲染器内部的原生 `table`，业务交互统一使用现有 shadcn/ui/Radix 组件；官方 CLI 识别 19 个已安装组件。 |
| AC-002-2 | passed | 亮暗主题的语义令牌对比度测试通过；`prefers-reduced-motion` 将动画与过渡收敛到 0.01ms；桌面、移动与 Radix 抽屉执行 WCAG 2 A/AA axe 扫描均为 0 violations，键盘焦点锁定和 Escape 关闭通过。 |
| AC-002-3 | passed | PublicHeader、AuthShell 与 TopNav 分别覆盖公开、认证和工作台容器；全部顶层工作台页面复用 TopNav，并使用统一 Skeleton、Empty、Alert、Dialog 和反馈模式。 |

## 自动化验证

```bash
cd backend
make ci

cd ../frontend
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build
npx shadcn@latest info

cd ..
docker compose -f docker-compose.yml config --quiet
docker compose --env-file .env.prod \
  -f docker-compose-prod.yml config --quiet
git diff --check
```

前端主题专项测试覆盖保存主题恢复、系统主题回退、切换持久化、亮暗对比度、减弱动效和两套 Header 的主题入口。完整前端测试共 34 个文件、109 个用例；Next.js 生产构建产出 19 个路由。

## 浏览器验收

使用 `agent-browser` 对当前实现执行真实交互：

- 以相同桌面视口对照 Vercel 首页与 HotKey 首页，确认克制排版、大留白、三栏 Hero、细边界和中性层级保持一致设计方向。
- 亮色切换暗色、页面刷新后持久化、系统主题回退和暗色雷达资产适配通过。
- 公开首页、登录、工作台桌面/移动布局通过；移动首页与工作台抽屉均可打开、键盘循环聚焦并由 Escape 关闭。
- 概览、告警、采集内容、事件、订阅、账户、监控与来源页面逐页执行 WCAG 2 A/AA axe 扫描，均为 0 violations。
- 浏览器控制台无应用 JavaScript 异常；只出现 Next.js 开发服务器 HMR 与 React DevTools 提示。

## 失败、降级与回滚

- 未保存主题时跟随操作系统偏好；localStorage 不可用时首屏脚本安全降级为服务端亮色标记，页面功能不受影响。
- 本条未修改 Schema、OpenAPI 或后端行为。回滚恢复 ThemeProvider、语义令牌、Header 入口和 shadcn Tailwind v4 配置即可，不涉及数据迁移。
