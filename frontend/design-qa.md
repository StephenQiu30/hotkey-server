# Vercel 风格与前端元素规范 Design QA

## 审计范围

- 审计日期：2026-08-13
- 技术基线：Next.js 16.3、React 19、Tailwind CSS v4、Radix、shadcn/ui 组合组件。
- 页面范围：公开首页、登录/注册入口、工作台共享导航、热点雷达及其桌面/移动、亮色/暗色状态。
- 规范范围：项目 002 设计系统、Vercel React/Next.js 实践、语义 HTML、键盘访问、状态表达、响应式与加载性能。
- 验证视口：1440 × 1000、390 × 844。

## 审计发现与处理

| 优先级 | 原问题 | 处理结果 |
|---|---|---|
| P1 | Tailwind 字体令牌写死 `Geist` 名称，没有绑定 `next/font` 生成的变量，无法保证实际使用自托管字体。 | `--font-sans`、`--font-mono` 已改为引用 `--font-geist-sans`、`--font-geist-mono`。 |
| P1 | 工作台共享布局和多个路由页同时输出 `<main>`，形成嵌套主地标；公开页和工作台都没有跳过导航入口。 | `BasicLayout` 成为唯一工作台主地标；新增可聚焦的“跳到主要内容”入口，公开首页同步接入。 |
| P1 | Card 默认没有边界，且全局 CSS 会无条件抹掉 CardHeader 的分隔线，导致表格、治理和筛选区域层级不稳定。 | Card 恢复统一 1px 语义边界和 flex 容器；移除覆盖分隔线的全局规则。 |
| P1 | Input、Select 与 outline Button 主要依赖浅色填充区分边界，暗色、复杂背景和禁用状态下识别度不足。 | 表单控件统一使用 `border-input`、显式焦点环和禁用背景；outline Button 恢复真实边框。 |
| P2 | Badge 带有不可点击的 hover 反馈，且重要性、质量状态缺少语义色映射。 | Badge 改为语义 `span`，移除误导性 hover；增加 success/warning 状态并应用于热点卡片。 |
| P2 | 热点筛选区没有区域标题，搜索、来源、监控、排序和日期在桌面宽度下缺乏稳定分组；空态没有直接恢复操作。 | 新增具名筛选区域、说明和顶部清除操作；采用 12 列布局；筛选空态提供“清除筛选”。 |
| P2 | 加载图标没有可读状态文本，监控状态只靠圆点颜色表达。 | 加载状态增加 `role=status`、`aria-live` 与屏幕阅读器文本；监控圆点增加文字状态。 |
| P2 | 暗色桌面登录页的雷达位图形成浅色矩形，介绍文案同时进入图片区域。 | 图片接入暗色反转，收敛图片宽高与文案列宽；认证表单统一使用设计系统 Card。 |
| P3 | 桌面/移动导航缺少稳定的底部分隔，移动触控控件尺寸不统一。 | 共享 Header 增加细边界；粗指针设备的按钮与表单控件至少保持 44px 触控高度。 |

## React 与 Next.js 实现检查

- 继续使用根布局中的 `next/font` 和首页的 `next/image`，未增加手工字体请求或原生图片加载。
- `useSearchParams` 仍位于已有 Suspense 边界内；本次没有扩大客户端组件边界。
- 热点、来源和监控请求继续并行加载，没有新增请求瀑布或串行 Effect。
- 退出登录改为 Next Router 导航，避免强制整页刷新。
- 共享 UI 修改集中在 Button、Card、Input、Select、Badge，没有为单页复制基础组件。

## 浏览器验收

- 首页桌面/移动的亮暗主题、移动抽屉、CTA 与主题切换通过。
- 未登录访问 `/dashboard/contents` 会保留 redirect 参数并返回登录页。
- 使用独立浏览器会话模拟本地 API 响应进入受保护热点雷达；没有修改后端、数据库或持久会话。
- 热点雷达在 1440px 与 390px 下均无横向溢出，统计卡、筛选区、状态 Badge、热点卡片和分页顺序一致。
- 登录页桌面暗色问题复验通过；移动登录/注册表单保持单列且无裁切。
- 浏览器 `errors` 无应用 JavaScript 异常。开发模式左下角 Next.js 工具按钮不属于生产 UI。

### 视觉证据

- [热点雷达桌面](../docs/acceptance/assets/038-hotspot-radar-desktop.png)
- [热点雷达移动端](../docs/acceptance/assets/038-hotspot-radar-mobile.png)
- [认证页暗色桌面](../docs/acceptance/assets/038-auth-dark-desktop.png)
- [首页暗色移动端](../docs/acceptance/assets/038-home-dark-mobile.png)

## 自动化验证

```bash
cd frontend
npm ci
npm run openapi:check
npm run typecheck
npm run test:unit
npm run build
```

- OpenAPI 生成客户端一致性：通过。
- TypeScript：通过。
- Vitest：45 个测试文件、194 个测试通过。
- Next.js 生产构建：通过，生成 20 个路由。
- `git diff --check`：通过。

## 验收结论

本轮发现的 P1/P2 设计系统、语义结构、响应式和暗色主题问题均已修复并回归。当前页面继续保持 Vercel 风格的中性颜色、清晰层级、细边界与克制动效，同时保留 HotKey 自有的信息结构和热点状态语义。

final result: passed
