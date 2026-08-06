---
layer: Design
scope: frontend
doc_no: "002"
title: Vercel风格与shadcn-ui设计系统设计
status: accepted
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/design/002-Vercel风格与shadcn-ui设计系统设计.md
prd: docs/prd/002-Vercel风格与shadcn-ui设计系统.md
plan: docs/plans/002-Vercel风格与shadcn-ui设计系统计划.md
---

# Vercel风格与shadcn-ui设计系统设计

## 现状

前端已经使用 Next.js、TypeScript、Tailwind、Radix 和一批 shadcn/ui 组件，并完成初步 Vercel 风格改版，但页面组合、状态覆盖和视觉令牌仍不完全统一。

## 目标

以 vercel.com/home 的克制排版、网格、留白、黑白层级和清晰信息密度为参考，以 shadcn/ui 官方组件组合为主要实现方式。

## 非目标

本条不跨越相邻编号实现其业务细节，也不建立第二套身份、任务、数据、API 或 UI 事实源。

## 核心决策

- 视觉参考用于信息层级与交互品质，不复制 Vercel 商标、文案、插画或受保护资产。
- 组件优先级为 shadcn/ui 官方组件→Radix primitive→少量业务组合组件；禁止为单页重复造 Button、Dialog、Table。
- Geist 字体、语义 CSS 变量、8px 间距节奏、1px 边界和有限圆角构成基础令牌；亮暗主题均满足 WCAG AA。
- 动效只表达状态变化，支持 prefers-reduced-motion；移动端从内容优先而非桌面缩放得到。

## 数据、接口与交互

- 颜色、字体、间距、圆角和阴影令牌
- 导航、表单、表格、筛选、对话框、空态和反馈组合规范
- 桌面/平板/移动断点
- 正常、加载、空、错误、权限不足状态

所有新增跨端字段先进入 `backend/db/schema.sql` 和后端 DTO/Swaggo 注解，再生成 `docs/openapi/swagger.json` 与前端 Client。页面同时提供正常、加载、空、错误和权限不足状态。

## 安全、失败与降级

过度追求像素复刻会牺牲产品语义；评审以一致性、可访问性和任务完成效率为准。

失败必须被分类、可观测且不破坏历史事实；可选外部依赖不可用时，核心读取链路继续工作。

## 设计验收边界

- 业务页面不再自建可由 shadcn/ui 提供的基础组件
- 键盘、焦点、标签、对比度和减弱动效通过检查
- 全部现有页面使用统一 Header、容器、状态与反馈模式
