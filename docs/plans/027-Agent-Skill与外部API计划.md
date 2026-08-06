---
layer: Plan
scope: shared
doc_no: "027"
title: Agent-Skill与外部API计划
status: planned
version: v1.0
owner: HotKey Team
phase: P1
canonical_path: docs/plans/027-Agent-Skill与外部API计划.md
design: docs/design/027-Agent-Skill与外部API设计.md
prd: docs/prd/027-Agent-Skill与外部API.md
---

# Agent-Skill与外部API计划

## 前置门禁

- Design 已 accepted，PRD 已 approved，依赖编号达到所需状态。
- 先建立当前实现清单和可复现差距；行为变化保存失败测试。
- UI 开发前确认真实 OpenAPI 能力，禁止用 Mock 掩盖缺失后端。

## 预计变更范围

- backend/internal/modules/identity/
- backend/internal/platform/http/
- backend/db/schema.sql
- docs/openapi/swagger.json
- skills/或仓库约定的集成目录
- backend/test/

## 执行步骤

1. 将 AC-027-* 转为领域、应用、Transport、前端组件和浏览器场景测试。
2. 在唯一 Schema 中做最小、向前兼容的数据变化并完成仓储实现。
3. 按模块依赖实现用例、持久任务、权限、审计、指标和错误映射。
4. 更新 Swaggo 注解并生成 OpenAPI，再生成前端 Client 和验证漂移。
5. 以 shadcn/ui 官方组件组合完成页面，不重复实现基础控件。
6. 验证并发、幂等、降级、配额、移动端、可访问性和完整用户路径。
7. 新增同编号 Acceptance；部署或人工流程需要长期执行时新增 Operations。

## 验证

- `cd backend && make ci`
- `cd frontend && npm run openapi:check && npm run typecheck && npm run test:unit && npm run build`
- 启动完整环境，以浏览器验证登录→监控→采集→事件→告警→报告主故事。
- 根目录运行开发/生产 Compose 配置检查及 `git diff --check`。

## 迁移与回滚

新行为通过配置或 capability 逐步启用。回滚先关闭新入口和任务生产者，等待/取消可重试任务，再回退应用；保留事实、审计、投递和模型运行记录。

## 依赖与功能切片

- 前置编号：003、004、021、025。
- 依赖未验收时，本条只允许完成不改变对外行为的基础工作。

- [ ] Slice-027-1：实现「服务令牌生命周期」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-027-* 回归。
- [ ] Slice-027-2：实现「Agent 专用权限 scope」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-027-* 回归。
- [ ] Slice-027-3：实现「热点、事件、证据和报告工具定义」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-027-* 回归。
- [ ] Slice-027-4：实现「手动搜索/告警动作与配额反馈」，补齐实际涉及的领域、任务、API 与界面，并绑定 AC-027-* 回归。

## 完成定义

- 撤销令牌立即失效且日志不暴露令牌
- Agent 无法越权读取或执行管理员动作
- 工具输出中的内容不能改变系统指令或调用权限
