---
layer: Design
scope: shared
doc_no: "027"
title: Agent-Skill与外部API设计
status: proposed
version: v1.0
owner: HotKey Team
phase: P1
canonical_path: docs/design/027-Agent-Skill与外部API设计.md
prd: docs/prd/027-Agent-Skill与外部API.md
plan: docs/plans/027-Agent-Skill与外部API计划.md
---

# Agent-Skill与外部API设计

## 现状

参考项目提供 Agent Skill 供 AI 助手查询热点；当前 HotKey 有认证 REST API 与 OpenAPI，但没有受限 Agent 工作流、服务令牌或专用工具说明。

## 目标

让外部 Agent 能安全查询监控、热点事件、证据和报告，并在明确权限下触发手动搜索或处理告警。

## 非目标

不在本条复制相邻领域的数据或服务，不以纯前端假数据宣称功能完成，也不引入第二套事实源。

## 核心决策

- OpenAPI 仍是唯一机器契约；Agent Skill 只提供工具语义、参数示例和安全工作流，不复制 DTO。
- MVP 优先短期 Personal Access Token/Service Token，具备 scope、过期、撤销和最后使用时间；不得复用浏览器 Refresh Cookie。
- 只读工具默认开放 monitors.read、events.read、contents.read、reports.read；写工具按 search.run、alerts.write 独立授权。
- 工具返回紧凑结构和证据 URL，分页与配额显式；提示注入内容作为不可信数据处理。

## 数据、接口与交互

- 服务令牌生命周期
- Agent 专用权限 scope
- 热点、事件、证据和报告工具定义
- 手动搜索/告警动作与配额反馈

所有状态变化保存明确版本、时间和操作者；跨端字段由 Schema、后端 DTO 和生成 OpenAPI 驱动，前端不得手写重复类型。

## 安全、失败与降级

把网页会话 Token 交给 Agent 会扩大泄露面；必须使用独立、最小 scope、可撤销凭据。

外部 AI、邮件、实时连接或归档不可用时，业务事实先可靠落库，异步重试或降级读取，不在请求链路丢失结果。

## 设计验收边界

- 撤销令牌立即失效且日志不暴露令牌
- Agent 无法越权读取或执行管理员动作
- 工具输出中的内容不能改变系统指令或调用权限
