---
layer: Plan
scope: fullstack
doc_no: "027"
title: Agent Skill与外部API计划
status: completed
version: v1.1
owner: HotKey Team
phase: P0
canonical_path: docs/plans/archive/027-Agent-Skill与外部API计划.md
design: docs/design/archive/027-Agent-Skill与外部API设计.md
prd: docs/prd/archive/027-Agent-Skill与外部API.md
---

# Agent Skill与外部API计划

## 前置门禁

- [x] 001–026 已有 Acceptance；027 Design v1.1 accepted、PRD v1.1 approved。
- [x] 审计会话认证、角色、现有业务路由、配额、审计、OpenAPI、Profile 与生成 Client。
- [x] 确认不把 Agent Token 接入原业务路由，而以独立别名复用 Handler/DTO，避免默认扩大权限。

## 预计变更范围

- backend/db/schema.sql
- backend/internal/modules/agentaccess/
- backend/internal/platform/http/
- backend/internal/modules/{monitor,event,ingestion,report,source,alert}/transport/http/
- backend/internal/bootstrap/
- backend/test/_suite/
- frontend/src/app/dashboard/profile/
- frontend/src/components/agent/
- frontend/src/services/hotkey/hotkey-server/
- frontend/test/
- skills/hotkey-agent/
- docs/openapi/

## 执行步骤

1. [x] 先补 Agent Token 领域校验、Repository/Service、Schema 约束、认证与 Scope 矩阵的失败测试。
2. [x] 实现 Token 创建/列表/撤销、哈希存储、过期/撤销/用户状态校验、最后使用时间和安全审计。
3. [x] 为六类能力挂载 `/api/v1/agent/*` 别名，复用现有 Handler/DTO；补全 Scope、角色降级、配额、版本冲突与越权测试。
4. [x] 生成 OpenAPI 与前端 Client，使用 shadcn/ui 在 Profile 实现 Token 全状态、一次性密钥 Dialog 和撤销确认。
5. [x] 使用官方 `skill-creator` 脚手架创建 `skills/hotkey-agent`，只保留必要 SKILL、OpenAI 元数据和 API 工作流参考并运行快速校验。
6. [x] 运行 OpenAPI、后端 CI、前端类型/全部单测/生产构建、Compose、架构与差异门禁。
7. [x] 使用 `$agent-browser` 验收 Viewer/Editor 的 Token 生命周期、真实 Agent API、拒绝矩阵、错误恢复、三视口、键盘和 WCAG。
8. [x] 写入 027 Acceptance，归档 Design/PRD/Plan，并以 `feat(agent): 完成Agent令牌与外部API` 独立提交。

## 验证

- `cd backend && make ci`
- `cd frontend && npm run openapi:check && npm run typecheck && npm run test:unit && npm run build -- --webpack`
- `python /Users/stephenqiu/.codex/skills/.system/skill-creator/scripts/quick_validate.py skills/hotkey-agent`
- 根目录执行开发/生产 Compose 配置检查与 `git diff --check`。
- 在隔离全栈环境用真实 Viewer/Editor、Agent Token 和 agent-browser 完成允许/拒绝故事。

## 迁移与回滚

Schema 只新增 `agent_tokens`，不改已有事实；回滚应用后该表可保留且无调用者。若需删除，必须先撤销全部 Token 并单独执行受控迁移。前端与 Skill 可随应用提交回退，不影响浏览器会话和业务数据。

## 依赖与功能切片

- 前置编号：003、004、005、017、021、023、024、025、026。
- [x] Slice-027-1：Token 生命周期——Schema、哈希、Scope/角色上限、到期、撤销、最后使用、审计。
- [x] Slice-027-2：外部 API——独立 Agent 路由、现有 DTO 复用、读取与低风险动作、拒绝矩阵。
- [x] Slice-027-3：Profile——生成 Client、全状态、一次性密钥、撤销确认、响应式与可访问性。
- [x] Slice-027-4：Skill——官方脚手架、工作流/错误/安全参考、快速校验和真实调用。
- [x] Slice-027-5：回归与验收——全量门禁、三视口、WCAG、安全日志和 Acceptance。

## 完成定义

- AC-027-1–8 均有自动化或浏览器证据，六 Scope 与非 Agent 路由拒绝矩阵完整。
- 原文/哈希无泄漏，生成物无漂移，Skill 官方校验通过，临时服务/浏览器/密钥资产已清理。
- 工作树只含 027 变更，并创建一个符合中文 Conventional Commit 规范的独立提交。
