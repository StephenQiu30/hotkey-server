---
layer: PRD
doc_no: "12"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:admin"
purpose: "定义管理员 API、任务观测、配置状态、审计日志与运维检查。"
canonical_path: "docs/product/prd/12-管理员API与运维观测PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "管理员API与运维观测需求边界"
  - "管理员API与运维观测TDD验收标准"
triggers:
  - "管理员API与运维观测范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/12-管理员API与运维观测实现计划.md"
---

# 12-管理员API与运维观测 PRD

## 1. 背景

管理员需要通过 API 管理来源、频道、任务状态、日报重跑和系统健康。

## 2. 目标

提供 server-only 管理员 API 和运维观测能力。

## 3. 范围

- 管理来源和频道。
- 查看采集运行记录。
- 查看任务状态和重试任务。
- 重跑日报。
- 查看 PostgreSQL、Redis、DashScope、SMTP 配置状态。
- 记录管理员审计日志。

## 4. 非目标

- 不做 Web 管理页面。
- 不做复杂 RBAC。
- 不做计费或租户管理。

## 5. 用户故事

- 作为管理员，我可以停用失败来源。
- 作为管理员，我可以重试失败任务。
- 作为管理员，我可以查看外部依赖是否配置正常。

## 6. 数据与 API 边界

数据表：`audit_logs`，并读取 `jobs`、`collection_runs`、`sources`、`channels`。

API：`/api/v1/admin/*`。

## 7. 后台任务影响

管理员 API 可以入队重试任务、日报重跑任务或来源测试采集任务。

## 8. 配置影响

管理员 API 依赖 `users.role=admin` 和认证中间件。

## 9. 错误与降级

外部依赖不可用时 health/status API 返回 degraded，不泄漏 secret。

## 10. 安全与合规

所有管理员写操作必须写 audit_logs，普通 user 访问返回 `403`。

## 11. 验收标准

- Given admin token，When 创建来源，Then 成功并写 audit log。
- Given user token，When 访问管理员 API，Then 返回 `403`。
- Given Redis 不可用，When 查询系统状态，Then 返回 degraded。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/12-管理员API与运维观测PRD.md
2. Read Plan: docs/plans/12-管理员API与运维观测实现计划.md
3. Write failing test first
4. Run expected failing command
5. Implement minimal code
6. Run required verification
7. Update OpenAPI or migrations when needed
8. Commit with Chinese message
9. Report commands, results, risks, and changed files back to Linear
```

Symphony 在本地 `Agents` 目录监听 Linear issue，并在独立 workspace 中执行。HotKey 不重写 Symphony 规范，只在 `WORKFLOW.md` prompt 中约束执行行为。

## 14. PRD 自审清单

- 本 PRD 是否只覆盖一个 feature。
- 用户、管理员或系统任务的输入输出是否明确。
- 范围和非目标是否能阻止越界实现。
- 数据、API、任务和配置影响是否可拆成 Plan。
- 验收标准是否可测试、可自动化、可在 harness 中执行。
- 是否遵循 TDD，且不要求先写生产代码。

## 15. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-31 | StephenQiu30 | 1.0.0 | 初版，按 server-only AI 热点检测与日报服务 feature 拆分创建 |

