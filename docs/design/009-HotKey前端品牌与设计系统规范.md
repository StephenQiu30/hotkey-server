# HotKey 前端品牌与设计系统规范

**状态：** 当前前端品牌与设计规范。Next.js App Router 初始化及生产容器迁移验证已完成；现有工作台业务界面仍待按本文规范逐页设计与迁移，公开 SEO 页面也尚未实现。验证细节见 [009 前端初始化验收](../acceptance/009-Next前端初始化与容器迁移验收.md)。

## 1. 范围与依据

本规范适用于 `hotkey-server/frontend/` 内的公开网页和 Web 工作台。Flutter 移动端 `app` 暂不实现。本规范是 HotKey 自己的设计规范，不代表 Vercel 官方产品，也不使用 Vercel 品牌资产。

参考来源：

- 仓库根目录 [`design.md`](../../design.md) 是 Vercel 品牌指南，适用于 Vercel 官方撰写的报告网站。该文件要求使用 Vercel wordmark、三角标记和其报告样式基础；HotKey 只借鉴其中清晰、克制、以读者任务为先、以证据支撑内容的通用原则，不复制 Vercel 标志、CSS、品牌身份或“Vercel 撰写”外观。
- [Vercel 官网首页](https://vercel.com/home)（2026-09-17 实际查看）：高对比中性色、简洁导航、单一主标题、少量明确行动入口和大量留白；之后通过产品演示与客户案例展开内容。其具体三角图形、文案、客户标志和版式不复制到 HotKey。
- [Geist Design System](https://vercel.com/geist/introduction)：参考 Geist 字体、颜色对比、网格和组件状态方面的系统思路。
- [Vercel Web Interface Guidelines](https://vercel.com/design/guidelines)：参考语义化交互、响应式排版、键盘操作、可访问名称和完整状态设计。

设计原则：先说明 HotKey 能帮助用户完成什么，再提供最短清晰的下一步。每个页面优先突出一个主要任务或结论；控制首屏信息量，避免把指标、重复摘要和卡片堆成仪表盘。需要证据时呈现来源、时间和限制，不夸大尚未验收的平台覆盖或实时能力。

## 2. 品牌与 Logo

- 产品名称固定写作 `HotKey`，大小写不可变更；作为品牌名时设置 `translate="no"`。
- 唯一母版标志为 [`frontend/public/logo.svg`](../../frontend/public/logo.svg)。图形以字母 H 为骨架，橙色上升连线表达热点变化，信号点表达持续发现。标志采用透明背景、黑色主体和单一暖橙色信号色，确保小尺寸下仍可辨识。Next.js 页面通过 Metadata API 显式引用此公共静态资源；不要复制出第二份标志文件。
- 导航栏优先采用“图形标志 + HotKey 文字”的横向组合；不要在 SVG 外叠加第二个图标、渐变或装饰框。Logo 周围至少保留一个 H 主笔画宽度的留白。独立标志建议不小于 20 × 20 CSS px；浏览器标签页可按方形 SVG 图标使用。
- 标志只作等比缩放，不拉伸、不旋转、不拆分、不添加阴影或描边，也不改造为类似 Vercel 三角形。深色背景上不要用 CSS 反相滤镜；如需暗色版本，另建并评审独立 SVG 变体。
- 导航链接同时提供可见的 `HotKey` 字样。只有图形的链接必须提供可访问名称（例如 `aria-label="HotKey 首页"`）；若可见文字已提供名称，图形本身可用空 `alt` 隐藏为装饰。

## 3. 技术栈与现状

未来前端目标栈由用户确定为：

- Next.js（优先 App Router，以服务端渲染和静态生成承载可索引的公开内容）
- TypeScript
- Tailwind CSS
- shadcn/ui + Radix UI
- ESLint + Prettier

以 shadcn/ui 管理可维护的组件源码与主题，以 Radix UI 承担复杂交互的键盘、焦点和无障碍行为；通过语义化 Tailwind token 统一样式。需要新组件时先复用现有 shadcn/ui/Radix 实现，不再引入第二套组件库或另造相同交互。

**当前检出状态：** `frontend/` 已初始化为 Next.js App Router + TypeScript + Tailwind CSS v4，含 shadcn/ui CLI 与 Radix UI 配置、ESLint、Prettier。现有工作台组件暂由客户端边界承载，业务界面尚未完成本文规定的设计迁移。Next.js `metadata`、robots 元数据和 nonce CSP 已配置；在公开页面与可索引事件页面实际实现前，不得声称 SEO 页面已交付，也不得将私有工作台加入 sitemap。

## 4. 视觉语言

### 色彩

- 以白色/近白背景、近黑正文和中性灰辅助信息构成默认主题；边框和分隔线轻而清楚。
- 暖橙 `#c2410c` 是 HotKey 的信号色，只用于 Logo 信号点、明确的热点/活动状态及少数关键强调。状态必须同时有文字或图标语义，不能只靠颜色表达。
- 错误、成功、警告分别使用语义色 token；不以品牌橙代替状态色。支持深色主题时使用相同语义 token 的深色值，不对整页套滤镜。
- 组件使用 `background`、`foreground`、`muted`、`muted-foreground`、`border`、`primary`、`destructive` 等语义 CSS 变量，避免在组件中散落硬编码颜色。

### 字体与布局

- 拉丁字母使用 Geist Sans；代码、来源 ID、时间戳等窄义标识才使用 Geist Mono。中文优先使用系统中文无衬线字体回退；字体加载失败时布局应保持稳定。
- 先用字号、字重、行距、对齐和留白建立层级。正文保持易读行宽；主标题每个页面只设一个。导航、按钮和正文使用简洁明确的产品文案。
- 桌面使用清晰的共享网格与对齐关系；窄屏逐级收拢为单栏，避免横向溢出。卡片仅用于有边界的独立对象，不把每段文字都包进盒子。
- 首屏围绕一个关键说明和一个主操作组织；其余内容按用户决策顺序展开。空白用于突出重点，不用大量空模块伪装内容。

### 交互与状态

- 优先 HTML 语义和 Next.js `<Link>`；交互控件使用 shadcn/ui/Radix 的已验证键盘与焦点行为。
- 所有交互提供 hover、focus-visible、active、disabled、loading、empty、error 和 success 状态。焦点清晰可见，操作结果有明确反馈；尊重 `prefers-reduced-motion`。
- 对话框、菜单、选择器和标签页必须保留 Radix 对应的键盘操作、可访问名称、焦点管理和关闭行为。不要用不可访问的 `div` 模拟按钮或链接。
- 事件状态、趋势和来源覆盖必须来自真实业务数据；没有可验证数据时显示空状态或限制说明，不填充虚构指标或模拟实时数值。

## 5. SEO 与公开内容

- 公开入口使用 Next.js Metadata API 和可被爬虫读取的服务端 HTML；每个可索引路由提供准确且唯一的标题、描述和规范 URL。使用 Open Graph 信息让分享卡片准确表达页面内容。
- 配置 `robots.txt` 与 `sitemap.xml`（Next.js `robots.ts`、`sitemap.ts` 等文件约定）；只列入实际存在、允许公开索引的稳定 URL。
- 公开文案围绕真实产品用途和用户问题编写，解释热点发现、事件脉络、评论研究和知识检索；不堆砌关键词，不把未实现能力写成服务承诺。
- 登录后工作台、个人监控配置、搜索参数、未授权的来源内容不得进入公开索引或站点地图。只有经过产品权限和隐私确认的公开事件页面才能建立可索引路由。
- 在生产域名确定后再配置 `metadataBase` 和 canonical 地址；不得把 localhost、临时预览域名或猜测的域名写入正式 SEO 元数据。

## 6. 目录与工程约定

- Next.js 路由按 App Router 的 `app/` / `src/app/` 组织；可复用 shadcn/ui 组件位于项目配置指定的 `components/ui/`，产品组件按业务职责收拢，不建立无职责的全局 `shared` 大杂烩。
- shadcn/ui 初始化配置与主题由 `components.json`、全局样式和 Tailwind 语义变量维护；组件变体在现有组件系统中表达，不另建一套与 shadcn 冲突的 token。
- TypeScript 严格模式。提交前运行 ESLint、Prettier 检查和 Next.js 生产构建；改动页面后用浏览器核验桌面、移动断点和主要交互状态。
- SEO 页面验收需检查返回 HTML 中的标题与正文、metadata、canonical、Open Graph、robots 和 sitemap；只检查浏览器客户端渲染不足以作为验收。

## 7. 官方参考

- [Vercel 官网首页](https://vercel.com/home)
- [Geist Design System](https://vercel.com/geist/introduction)
- [Vercel Web Interface Guidelines](https://vercel.com/design/guidelines)
- [Next.js Metadata 与 Open Graph](https://nextjs.org/docs/app/getting-started/metadata-and-og-images)
- [Next.js Metadata API](https://nextjs.org/docs/app/api-reference/functions/generate-metadata)
- [Next.js App Icon 文件约定](https://nextjs.org/docs/app/api-reference/file-conventions/metadata/app-icons)
- [Next.js Sitemap](https://nextjs.org/docs/app/api-reference/file-conventions/metadata/sitemap)
- [shadcn/ui Next.js 安装指南](https://ui.shadcn.com/docs/installation/next)
- [Radix UI Accessibility](https://www.radix-ui.com/primitives/docs/overview/accessibility)
