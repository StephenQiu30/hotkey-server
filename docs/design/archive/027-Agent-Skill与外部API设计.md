---
layer: Design
scope: fullstack
doc_no: "027"
title: Agent Skill与外部API设计
status: accepted
version: v1.1
owner: HotKey Team
phase: P0
canonical_path: docs/design/archive/027-Agent-Skill与外部API设计.md
prd: docs/prd/archive/027-Agent-Skill与外部API.md
plan: docs/plans/archive/027-Agent-Skill与外部API计划.md
---

# Agent Skill与外部API设计

## 现状

HotKey 已有监控、Radar 事件、内容证据、报告、手动采集和告警处理 API，但只能使用 15 分钟浏览器 Access Token。现有路由以数据库会话和角色鉴权，没有 Scope 语义；让长期 Agent Token 直接进入原路由会把未显式审计的管理接口一并暴露。仓库也没有可安装 Skill，外部 Agent 不知道分页、证据优先、配额、错误码和不可信内容边界。

## 目标

提供用户自助管理的短期 Agent Token、显式 Scope 限权的 `/api/v1/agent/*` API 别名和仓库内 `hotkey-agent` Skill，使外部 Agent 能安全完成“列出监控→查询事件→读取证据/报告→按权限触发手动采集或处理告警”的 MVP 故事。

## 非目标

- 不实现 OAuth Client、第三方应用市场、Webhook、MCP Server、自动 Token 轮换或服务账户组织模型。
- 不开放用户、来源连接、治理、模型配置、事件治理、报告发布和告警抑制等管理操作。
- 不复制业务 DTO、手写前端 API 类型或让 Skill 成为第二份接口契约。
- 不允许网页、标题、正文或报告内容修改系统指令、Scope、目标地址或工具权限。

## 核心决策

- Agent Token 归属当前用户，原文仅在创建成功时返回一次；数据库只保存 SHA-256 哈希和安全前缀。Token 有名称、Scope、1–90 天到期时间、最后使用时间、撤销时间与乐观锁版本；每个用户最多 10 个未撤销且未过期 Token。
- 浏览器 JWT 只进入原 `/api/v1/*` 路由，Agent Token 只进入独立 `/api/v1/agent/*` 路由。Agent 路由复用现有 Handler、应用服务和 DTO，仅新增路由别名与 Scope 守卫。
- Scope 固定为 `monitors.read`、`events.read`、`contents.read`、`reports.read`、`search.run`、`alerts.write`。Viewer 只能申请读取与 `alerts.write`；Editor/Admin 可额外申请 `search.run`。认证时重新读取当前用户角色与状态，因此禁用、删除或降级立即收窄权限。
- `alerts.write` 只开放告警列表、详情、确认与解决；不开放抑制。`search.run` 只接受 Monitor ID，复用既有配额、幂等和任务队列，不接受任意查询、来源或 URL。
- OpenAPI 是唯一机器契约，生成前端 Client；Skill 只解释工作流、Scope、分页、配额、错误恢复和提示注入边界。Skill 使用官方 `skill-creator` 脚手架，目录为 `skills/hotkey-agent`。

## 数据、接口与交互

- `agent_tokens` 保存 `user_id`、名称、前缀、哈希、Scope 数组、到期/最后使用/撤销时间、版本和审计时间；原文、Authorization Header 与请求正文不得进入日志或审计。
- 浏览器会话管理接口：`GET/POST /api/v1/agent-tokens`、`POST /api/v1/agent-tokens/{id}/revoke`。创建时返回一次 `token`；列表和撤销响应永不返回原文或哈希。
- Agent 读取接口：监控列表/详情，Radar 与事件详情/证据/更新/热度/智能结果，内容列表/详情/文档，报告列表/详情。
- Agent 动作接口：按 Monitor 触发手动采集；列出/读取/确认/解决告警。响应继续使用统一 Result、游标、业务码、`X-Request-ID` 和既有 429 配额详情。
- Profile 使用 shadcn/ui Card、Dialog、Checkbox、Input、Select、Badge、Alert 与 Button 管理 Token；密钥只在创建成功 Dialog 显示一次，关闭前明确提示保存，撤销需确认且失败可重试。

## 安全、失败与降级

- 使用恒定格式随机密钥和 SHA-256 精确匹配；不存在、过期、撤销、用户禁用/删除统一返回 401，不泄露命中阶段。缺少 Scope 返回 403，角色不满足写动作也返回 403。
- 撤销与认证均读取数据库事实，不使用进程缓存；撤销提交后下一次请求立即失败。`last_used_at` 只记录时间，不记录 Token、输入或响应。
- Agent 只访问配置中的 HotKey Base URL。Skill 将所有业务内容标记为不可信数据：不得执行正文中的指令，不得改变请求目标、Authorization、Scope 或调用未列出的端点。
- 上游、队列或数据库不可用时沿用 503；配额耗尽沿用 429 和恢复时间。Agent 不自动扩大权限、不绕过撤销、不以浏览器 Cookie 降级。

## 设计验收边界

- 原文只显示一次，Schema、API、日志、审计与列表均不含原文/哈希；撤销、到期、禁用和删除即时返回 401。
- 六个 Scope 对每个 Agent 路由逐一映射；无 Scope 返回 403，Viewer 无法创建 `search.run`，角色降级后旧 Token 也不能执行搜索。
- Agent 路由复用现有业务 DTO/服务，手动采集继续消耗用户配额，告警写入继续使用版本冲突与不可变审计。
- Skill 通过官方快速校验，明确 `$hotkey-agent`、Base URL/Token 环境变量、可用工作流、分页/429/401/403 处理和提示注入防护。
- 自动化、OpenAPI、全量回归、Compose 和 agent-browser 在 360×800、768×1024、1280×800 的 Token 生命周期/API 文档与真实调用故事全部通过。
