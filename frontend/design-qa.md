# HotKey 方案 1 设计 QA

## 验收范围

- 设计源：`/Users/stephenqiu/.codex/generated_images/019fd68a-3f08-7801-9516-8fc6b945e154/exec-c0a18aaf-13e6-4dae-9daf-1191ff418e7e.png`
- 桌面实现：`.codex-design/hotkey-option1-final-1487x1058.png`
- 同屏对照：`.codex-design/hotkey-option1-comparison-final.png`
- 移动实现：`.codex-design/hotkey-option1-mobile-final-390x844.png`
- 桌面视口：1487 × 1058，可见首屏状态
- 移动视口：390 × 844，首页初始状态与导航展开状态

## 对照结论

- 字体与层级：Geist / Geist Mono 通过 `next/font` 统一加载；中文使用系统无衬线回退。首屏标题已调整为常规字重，并与设计源的字号、换行和基线对齐。
- 间距与布局：桌面首屏保持三栏结构，标题起点约为 `(92, 410)`，来源带起点为 `y=929`；雷达、主标题、辅助文案与 CTA 的垂直关系与方案 1 一致。
- 颜色与表面：全站切换为黑、白、中性灰令牌；主要按钮为纯黑，卡片使用细边框和低圆角，移除非必要阴影。
- 图像与图标：首屏与认证页使用真实生成的雷达 PNG；品牌标记使用 Lucide Radar 图标，未使用 CSS 绘图、占位框或手绘 SVG。
- 响应式：桌面 `scrollWidth === clientWidth`；移动端 `scrollWidth === clientWidth`，无横向溢出。导航抽屉可打开并可通过 Escape 关闭。
- 状态与交互：首页导航、主 CTA、简报锚点与报告入口均为真实链接；未登录访问工作台会回到登录页。认证页表单字段可见且布局稳定。
- 可访问性：主标题具备稳定的可访问名称；区域、导航、按钮和图片均有语义标签；焦点环与移动端点击目标由统一组件保证。
- 控制台：最终桌面与移动端新会话均无 warning / error；所有首屏图片均加载完成。

## 修正记录

1. P1 — 首轮首屏标题、雷达和来源带纵向基线低于设计源。通过重新分配首屏行高、扩大雷达视觉占比并调整桌面偏移完成修正。
2. P1 — 雷达缩放曾导致移动端 13px 横向溢出。将放大限制在 `lg` 断点，复验后移动端宽度完全吻合。
3. P2 — 原品牌图标在小尺寸下辨识度不足。改为同风格的 Lucide Radar 图标，提升导航与认证页的一致性。
4. P2 — 认证页首屏雷达触发 LCP 提示。将图片设为优先加载，最终新会话无控制台告警。

## 自动化验证

- TypeScript：通过
- OpenAPI 生成契约：通过
- Vitest：33 个文件、104 个测试通过
- Next.js 生产构建：通过，19 个页面生成完成
- `git diff --check`：通过

final result: passed
