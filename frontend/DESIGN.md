# HotKey Web 设计实现约束

仓库根 [DESIGN.md](../DESIGN.md) 保留完整的 Vercel 设计规范，是不可在本实现文件中改写的视觉参考。本文件只记录 HotKey Web 对该规范的组件映射、无边框取舍与工程约束。

## 1. 视觉基线

- 字体：Geist 用于界面与标题，Geist Mono 只用于技术标签、数据与代码。
- 主色：`#171717`；页面：`#ffffff`；柔和表面：`#fafafa` / `#f5f5f5`；正文：`#4d4d4d`；弱文字最低使用 `#666666` 以保持小字号可读性。
- 强调色仅使用根设计中的蓝青、紫粉、珊瑚琥珀三组渐变，并仅用于 Hero 或大面积信号表达。
- 间距、字号、行高、字距、圆角和容器只使用 Tailwind 命名尺度；标题最高使用 `font-semibold`，正文不用等宽字体。
- `src/app/globals.css` 是颜色、字体和圆角令牌的唯一实现入口。业务组件只使用 `bg-background`、`text-muted-foreground` 等语义令牌，不复制十六进制颜色。
- 禁止原始像素值和任意布局尺寸；项目中的 xxl 统一使用 Tailwind 官方 `2xl` 名称。

## 2. 无边框规则

“无边框”指默认信息层级不依赖可见描边：

- 页面区块、导航和静态信息卡使用留白、背景明度与排版形成分组。
- `Card` 默认不带边框、ring 或投影；浮层可使用 shadcn 默认的轻量 ring 与阴影，以表明遮盖关系。
- 输入框、选择器、错误态和键盘焦点必须保留轮廓。它们是操作和可访问性反馈，不按装饰边框删除。
- 表格只在行列辨识确有需要时使用 hairline；空态、骨架屏和只读摘要优先使用柔和表面。
- 同一页面只使用一种 CTA 圆角尺度。工作台使用 `rounded-md` / `rounded-lg`，营销 Hero 才可通过组件 variant 使用完整 pill。

## 3. 组件与状态

- 基础组件由 shadcn CLI 添加并保存在 `src/components/ui/`；底层固定为 Radix。
- 使用组件提供的 variant/size，图标来自 Lucide。按钮内图标设置 `data-icon="inline-start|inline-end"`。
- Card 使用 `CardHeader`、`CardTitle`、`CardDescription`、`CardContent`、`CardFooter` 等完整组合。
- 跨功能页面模式保存在 `src/components/patterns/`；路由只组合 feature 与 pattern，不复制完整页面状态。
- 每个业务切片在 `src/features/<feature>/` 实现正常、空、加载、部分、错误和无权限状态；禁止在页面中复制后端权限或业务规则。
- Umi OpenAPI 根据 Swagger/OpenAPI 快照把端点与类型直接生成到 `src/api/`，不增加 `generated/` 中间层；浏览器请求只调用这些生成文件，传输错误由 `src/request.ts` 统一承接。

## 4. 响应与可访问性

- base 使用手机优先单列；`sm` 允许紧凑双列；`md` 展开导航；`lg` 使用桌面结构；`xl` 与 `2xl` 增加外围留白并保持命名最大宽度。
- 交互触点通过 Button 等组件的语义 size variant 保证，页面不得写尺寸补丁。
- 所有交互支持键盘焦点；装饰图形使用 `aria-hidden`；导航和状态区域提供语义名称。
- 动效尊重 `prefers-reduced-motion`，不以动画作为唯一状态提示。
- 新页面必须在桌面和窄屏浏览器中实际检查，再记录为视觉验收证据。

## 5. 可用性与维护性

- App Router 提供 loading、error、global-error 与 not-found 边界，局部失败不得产生空白页。
- 统一 PageState 组件承接错误、空态、无权限和恢复操作；Skeleton 承接加载状态。
- `/health` 只证明 Web 进程可响应，后端依赖状态由后端 readiness 负责。
- `pnpm check:design` 拒绝原始像素值和任意布局尺寸；`pnpm check:boundaries` 拒绝反向依赖与跨 feature 直接引用。
- 生产镜像保持 standalone、非 root、只读文件系统，并通过容器健康检查暴露运行状态。

## 6. 品牌资产

`src/app/icon.svg` 是当前唯一图标母版，并由 Next.js Metadata 文件约定自动引用。不得使用 Vercel 标志、字样或其他品牌资产。后续替换母版时同步产品设计与各平台导出物。
