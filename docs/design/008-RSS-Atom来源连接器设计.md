---
layer: Design
scope: backend
doc_no: "008"
title: RSS-Atom来源连接器设计
status: proposed
version: v1.0
owner: HotKey Team
phase: P0
canonical_path: docs/design/008-RSS-Atom来源连接器设计.md
prd: docs/prd/008-RSS-Atom来源连接器.md
plan: docs/plans/008-RSS-Atom来源连接器计划.md
---

# RSS-Atom来源连接器设计

## 现状

RSS/Atom HTTPS 连接器、分页、ETag/Last-Modified、SSRF 防护、标准化入口和来源配置界面已经实现。

## 目标

将财新、36氪、澎湃、人民网、第一财经、界面新闻、虎嗅等拥有公开或授权 Feed 的站点作为首要稳定事实源。

## 非目标

本条不跨越相邻编号实现其业务细节，也不建立第二套身份、任务、数据、API 或 UI 事实源。

## 核心决策

- 一个 SourceConnection 绑定一个不可变 HTTPS Feed 根端点；翻页只能同源并重复执行安全校验。
- 支持 RSS 2.0 与 Atom，保留 guid/id、canonical URL、作者、发布时间、摘要、正文和附件元数据。
- 使用 ETag、Last-Modified 和 checkpoint 增量拉取；单次页数、正文大小、项目数和超时均有硬上限。
- 来源没有公开 Feed 时不抓取网页，改为等待授权 Feed 或由用户提供合法订阅地址。

## 数据、接口与交互

- RSS/Atom 解析和增量采集
- 条件请求、分页和检查点
- 正文与元数据标准映射
- 来源健康和诊断

所有新增跨端字段先进入 `backend/db/schema.sql` 和后端 DTO/Swaggo 注解，再生成 `docs/openapi/swagger.json` 与前端 Client。页面同时提供正常、加载、空、错误和权限不足状态。

## 安全、失败与降级

媒体站点 Feed 可随时变更；解析失败要保留诊断但不能回退到未授权爬虫。

失败必须被分类、可观测且不破坏历史事实；可选外部依赖不可用时，核心读取链路继续工作。

## 设计验收边界

- 重复窗口不会重复创建 Content
- 恶意地址、私网解析和跨域重定向被拒绝
- 无正文 Feed 可降级为摘要且标明证据完整度
