---
layer: PRD
scope: frontend
doc_no: "002"
title: Vercel风格与shadcn-ui设计系统
status: draft
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/prd/002-Vercel风格与shadcn-ui设计系统.md
design: docs/design/002-Vercel风格与shadcn-ui设计系统设计.md
plan: docs/plans/002-Vercel风格与shadcn-ui设计系统计划.md
---

# Vercel风格与shadcn-ui设计系统

## 用户问题

前端已经使用 Next.js、TypeScript、Tailwind、Radix 和一批 shadcn/ui 组件，并完成初步 Vercel 风格改版，但页面组合、状态覆盖和视觉令牌仍不完全统一。

## 产品目标

以 vercel.com/home 的克制排版、网格、留白、黑白层级和清晰信息密度为参考，以 shadcn/ui 官方组件组合为主要实现方式。

## 范围

- 颜色、字体、间距、圆角和阴影令牌
- 导航、表单、表格、筛选、对话框、空态和反馈组合规范
- 桌面/平板/移动断点
- 正常、加载、空、错误、权限不足状态

## 用户故事

- 作为普通用户，我能够看懂当前状态、结果来源和下一步动作。
- 作为编辑者，我能够在权限范围内完成配置或处理任务，并获得明确反馈。
- 作为管理员，我能够治理配置、失败、成本与审计，而不直接修改数据库事实。

## 功能要求

- FR-002-1：视觉参考用于信息层级与交互品质，不复制 Vercel 商标、文案、插画或受保护资产。
- FR-002-2：组件优先级为 shadcn/ui 官方组件→Radix primitive→少量业务组合组件；禁止为单页重复造 Button、Dialog、Table。
- FR-002-3：Geist 字体、语义 CSS 变量、8px 间距节奏、1px 边界和有限圆角构成基础令牌；亮暗主题均满足 WCAG AA。
- FR-002-4：动效只表达状态变化，支持 prefers-reduced-motion；移动端从内容优先而非桌面缩放得到。

## 非功能要求

- 所有写操作具备认证、授权、输入校验、幂等或乐观锁保护和审计。
- 列表支持稳定分页；第三方调用具备超时、配额、重试和降级。
- Web 覆盖桌面与移动端，并满足键盘操作、可见焦点和减弱动效。

## 非目标

不使用未授权抓取补齐来源能力，不为本条引入与仓库规范冲突的新基础设施。

## 验收标准

- AC-002-1：业务页面不再自建可由 shadcn/ui 提供的基础组件
- AC-002-2：键盘、焦点、标签、对比度和减弱动效通过检查
- AC-002-3：全部现有页面使用统一 Header、容器、状态与反馈模式
