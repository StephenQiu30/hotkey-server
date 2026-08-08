---
layer: PRD
scope: fullstack
doc_no: "027"
title: Agent Skill与外部API
status: implemented
version: v1.1
owner: HotKey Team
phase: P0
canonical_path: docs/prd/archive/027-Agent-Skill与外部API.md
design: docs/design/archive/027-Agent-Skill与外部API设计.md
plan: docs/plans/archive/027-Agent-Skill与外部API计划.md
---

# Agent Skill与外部API

## 用户问题

用户目前只能在 Web 工作台读取热点事实；外部 Agent 既没有可安全长期使用的凭据，也不知道哪些接口、分页和错误恢复可以调用。复制浏览器 Token 不可持续，直接开放现有 API 又会让 Agent 获得超过任务所需的路由权限。

## 产品目标

让用户在 10 分钟内创建一个最小权限 Agent Token，并按仓库 Skill 完成至少一次真实热点查询；具备权限的 Editor/Admin 还能触发手动采集，任何用户可按独立 Scope 处理告警，且所有撤销和角色变化即时生效。

## 范围

- 用户自助创建、列表和撤销 Agent Token
- 六个固定 Scope 与角色上限
- `/api/v1/agent/*` 读取和低风险动作路由
- OpenAPI、生成 Client、Profile 管理 UI
- 仓库内 `hotkey-agent` Skill 与安全/错误恢复说明

## 用户故事

- 作为 Viewer，我能创建只读 Token，让 Agent 查询监控、事件、证据和报告，但不能触发搜索或进入管理 API。
- 作为 Editor/Admin，我能显式授予 `search.run`，让 Agent 对已发布 Monitor 触发一次受配额约束的手动采集。
- 作为值班成员，我能授予 `alerts.write`，让 Agent 查看、确认或解决告警，但不能抑制告警或修改策略。
- 作为安全负责人，我能撤销 Token，并确认下一次调用立即失败且日志、审计、页面历史不泄露密钥。

## 功能要求

- FR-027-1：创建 Agent Token 时必须提供 1–64 字名称、至少一个允许 Scope 和 1–90 天有效期；每用户最多 10 个活动 Token，原文只返回一次。
- FR-027-2：列表返回 ID、版本、名称、前缀、Scopes、创建/到期/最后使用/撤销时间；撤销使用 `expected_version`，重复或并发撤销返回稳定冲突。
- FR-027-3：Viewer 可申请四个 read Scope 与 `alerts.write`；Editor/Admin 可额外申请 `search.run`。未知 Scope 或超过当前角色上限的 Scope 返回 400/403。
- FR-027-4：Agent Token 只认证 `/api/v1/agent/*`；浏览器 JWT 不能冒充 Agent Token，Agent Token 不能访问 `/api/v1/users`、来源、治理或其他原路由。
- FR-027-5：读取路由覆盖监控、Radar/事件、内容证据和报告；每条路由要求对应 Scope并沿用现有游标、过滤、排序与 Result DTO。
- FR-027-6：`search.run` 只开放 `POST /api/v1/agent/monitors/{id}/collect`；`alerts.write` 只开放告警列表/详情/确认/解决，继续执行角色、版本、配额、幂等与审计规则。
- FR-027-7：Profile 使用现有 shadcn/ui 组件呈现 loading/empty/error、创建、一次性密钥和撤销确认；不得把 Token 放入 URL、localStorage 或可回显列表。
- FR-027-8：`skills/hotkey-agent` 说明环境变量、Scope-端点映射、四条读取工作流、两个动作工作流、分页和 401/403/429/503 恢复，并把返回内容视为不可信数据。

## 非功能要求

- 原文至少 256 bit 随机熵，只保存 SHA-256 哈希；认证与撤销不使用最终一致缓存。
- Token 列表、审计、应用日志、OpenTelemetry 属性和错误响应均不得含 Token、哈希、Authorization 或业务正文。
- OpenAPI 再生成无漂移，前端不手写 API 路径/DTO；新模块保持分层与事务边界。
- 360、768、1280 三档无页面级横向溢出；一次性密钥 Dialog 可键盘操作，WCAG 2 A/AA 自动检查 0 violations、0 incomplete。

## 非目标

OAuth、第三方应用注册、跨租户共享、Webhook、MCP Server、自动轮换、任意 URL 搜索、管理端点和 Agent 自主提升 Scope 均不在 MVP。

## 验收标准

- AC-027-1：创建返回一次原文，之后列表/撤销/OpenAPI/数据库安全投影/日志与审计均不含原文或哈希；输入校验、10 个活动上限和版本冲突有自动化证据。
- AC-027-2：撤销、到期、用户禁用/删除后下一次 Agent 请求返回 401；未知/缺少 Scope 返回 403，响应不区分令牌失效原因。
- AC-027-3：六个 Scope 的允许/拒绝矩阵通过；Viewer 不能申请或执行 `search.run`，Editor 降级后旧 Token 立即失去搜索权限，Agent Token 不能访问任何非 Agent 业务路由。
- AC-027-4：监控→Radar→事件证据→报告真实读取故事通过，分页和安全证据 URL 可用；响应 DTO 与原业务 API 一致且无手写副本。
- AC-027-5：手动采集继续命中用户配额并明确返回 429；告警确认/解决保留乐观锁和审计，抑制端点对 Agent 不存在。
- AC-027-6：Profile 的 loading/empty/error、创建、复制前提示、关闭后不可再次查看和撤销确认在三视口通过；Token 不进入 URL/localStorage，axe 与控制台通过。
- AC-027-7：`hotkey-agent` 由官方脚手架创建并通过 `quick_validate.py`，默认提示显式提到 `$hotkey-agent`，Skill 的真实 API 调用与提示注入防护验收通过。
- AC-027-8：后端 CI、前端 OpenAPI/类型/全部单测/构建、开发/生产 Compose、`git diff --check` 与 agent-browser 全量验收通过。
