---
layer: PRD
scope: shared
doc_no: "027"
title: Agent-Skill与外部API
status: draft
version: v1.0
owner: HotKey Team
phase: P1
canonical_path: docs/prd/027-Agent-Skill与外部API.md
design: docs/design/027-Agent-Skill与外部API设计.md
plan: docs/plans/027-Agent-Skill与外部API计划.md
---

# Agent-Skill与外部API

## 用户问题

参考项目提供 Agent Skill 供 AI 助手查询热点；当前 HotKey 有认证 REST API 与 OpenAPI，但没有受限 Agent 工作流、服务令牌或专用工具说明。

## 产品目标

让外部 Agent 能安全查询监控、热点事件、证据和报告，并在明确权限下触发手动搜索或处理告警。

## 范围

- 服务令牌生命周期
- Agent 专用权限 scope
- 热点、事件、证据和报告工具定义
- 手动搜索/告警动作与配额反馈

## 用户故事

- 作为阅读者，我能快速看到重要变化、依据、可信程度和更新时间。
- 作为编辑者，我能从发现、研判到处理和交付完成闭环。
- 作为管理员，我能查看成本、失败、权限和审计并安全恢复。

## 功能要求

- FR-027-1：OpenAPI 仍是唯一机器契约；Agent Skill 只提供工具语义、参数示例和安全工作流，不复制 DTO。
- FR-027-2：MVP 优先短期 Personal Access Token/Service Token，具备 scope、过期、撤销和最后使用时间；不得复用浏览器 Refresh Cookie。
- FR-027-3：只读工具默认开放 monitors.read、events.read、contents.read、reports.read；写工具按 search.run、alerts.write 独立授权。
- FR-027-4：工具返回紧凑结构和证据 URL，分页与配额显式；提示注入内容作为不可信数据处理。

## 非功能要求

- 写操作具备认证授权、审计、幂等或乐观锁；异步操作可追踪和重试。
- 列表使用稳定分页，慢任务不阻塞 HTTP；错误使用统一业务码。
- 前端使用 shadcn/ui/Radix 组合，覆盖响应式、键盘、焦点、对比度与减弱动效。

## 非目标

不以静态 Mock、不可追溯模型文本或仅桌面可用页面通过验收。

## 验收标准

- AC-027-1：撤销令牌立即失效且日志不暴露令牌
- AC-027-2：Agent 无法越权读取或执行管理员动作
- AC-027-3：工具输出中的内容不能改变系统指令或调用权限
