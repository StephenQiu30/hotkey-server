---
layer: PRD
scope: backend
doc_no: "016"
title: DuckDuckGo-Instant-Answer边界
status: draft
version: v1.0
owner: HotKey Team
phase: P2
canonical_path: docs/prd/016-DuckDuckGo-Instant-Answer边界.md
design: docs/design/016-DuckDuckGo-Instant-Answer边界设计.md
plan: docs/plans/016-DuckDuckGo-Instant-Answer边界计划.md
---

# DuckDuckGo-Instant-Answer边界

## 用户问题

参考项目实现了 DuckDuckGo 搜索但未加入定时任务；DuckDuckGo 官方公开资料没有提供可作为通用有机网页搜索结果源的稳定开放 API。

## 产品目标

只在明确允许的 Instant Answer 或正式合作接口范围内提供补充答案，不将 DuckDuckGo 页面抓取包装成通用搜索连接器。

## 范围

- 能力边界说明和禁用态
- 可选 Instant Answer 映射
- 禁止热度计权规则
- 未来合作接口扩展点

## 用户故事

- 作为编辑者，我能选择真实可用的来源能力并预览查询结果。
- 作为管理员，我能配置授权、健康、配额和失败处理，而看不到明文密钥。
- 作为阅读者，我能识别原始事实、派生内容和不可用来源。

## 功能要求

- FR-016-1：MVP 不抓取 DuckDuckGo HTML 搜索结果，也不宣称 Instant Answer 等价于网页搜索。
- FR-016-2：如使用 Instant Answer，结果类型标记为 knowledge_answer，只参与实体补充，不计入多源热度或事件传播宽度。
- FR-016-3：传统网页链接需求改由 RSS、官方平台 API 或获授权的 Web Search provider 满足。
- FR-016-4：只有 DuckDuckGo 提供正式合作读取接口后，才新增可调度 Search capability。

## 非功能要求

- 外部请求必须具备固定端点、DNS/重定向复验、超时、限流和最大响应限制。
- 凭据只保存 `env:NAME` 引用；日志、错误、证据和前端不得暴露 Token。
- 采集、重试和人工触发保持幂等，可在 CollectionRun 中追踪。

## 非目标

不保证第三方未授权或已退役能力与参考项目保持表面一致。

## 验收标准

- AC-016-1：代码和 UI 不出现“DuckDuckGo 全网搜索已支持”的误导
- AC-016-2：Instant Answer 不创建虚假来源指标
- AC-016-3：未配置正式能力时不会产生调度任务
