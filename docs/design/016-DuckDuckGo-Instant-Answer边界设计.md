---
layer: Design
scope: backend
doc_no: "016"
title: DuckDuckGo-Instant-Answer边界设计
status: proposed
version: v1.0
owner: HotKey Team
phase: P2
canonical_path: docs/design/016-DuckDuckGo-Instant-Answer边界设计.md
prd: docs/prd/016-DuckDuckGo-Instant-Answer边界.md
plan: docs/plans/016-DuckDuckGo-Instant-Answer边界计划.md
---

# DuckDuckGo-Instant-Answer边界设计

## 现状

参考项目实现了 DuckDuckGo 搜索但未加入定时任务；DuckDuckGo 官方公开资料没有提供可作为通用有机网页搜索结果源的稳定开放 API。

## 目标

只在明确允许的 Instant Answer 或正式合作接口范围内提供补充答案，不将 DuckDuckGo 页面抓取包装成通用搜索连接器。

## 非目标

不使用未授权网页抓取、浏览器会话、验证码绕过或不稳定私有接口补齐能力；本条不实现其他编号的业务细节。

## 核心决策

- MVP 不抓取 DuckDuckGo HTML 搜索结果，也不宣称 Instant Answer 等价于网页搜索。
- 如使用 Instant Answer，结果类型标记为 knowledge_answer，只参与实体补充，不计入多源热度或事件传播宽度。
- 传统网页链接需求改由 RSS、官方平台 API 或获授权的 Web Search provider 满足。
- 只有 DuckDuckGo 提供正式合作读取接口后，才新增可调度 Search capability。

## 数据、接口与交互

- 能力边界说明和禁用态
- 可选 Instant Answer 映射
- 禁止热度计权规则
- 未来合作接口扩展点

跨端契约先修改 Schema 与后端注解，再生成 OpenAPI 和前端 Client。来源能力必须在发布监控前可检测，禁用或阻塞状态不能进入调度。

## 安全、失败与降级

用非官方端点追求参考项目表面一致会引入稳定性与合规风险；本条明确选择能力真实而非数量好看。

第三方失败按认证、限流、临时、解析和永久错误分类；关闭单个来源不影响历史证据和其他来源。

## 设计验收边界

- 代码和 UI 不出现“DuckDuckGo 全网搜索已支持”的误导
- Instant Answer 不创建虚假来源指标
- 未配置正式能力时不会产生调度任务

## 官方依据

- https://duckduckgo.com/duckduckgo-help-pages/features/instant-answers-and-other-features
- https://duckduckgo.com/duckduckgo-help-pages/results/sources
