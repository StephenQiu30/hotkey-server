---
layer: PRD
doc_no: "02"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:auth"
purpose: "定义邮箱账号、会话、user/admin 两级角色与管理 API 鉴权边界。"
canonical_path: "docs/product/prd/02-用户账号与角色PRD.md"
status: draft
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - "README.md"
  - "WORKFLOW.md"
outputs:
  - "用户账号与角色需求边界"
  - "用户账号与角色TDD验收标准"
triggers:
  - "用户账号与角色范围变更"
  - "对应 Linear issue 拆分或合并"
downstream:
  - "docs/plans/02-用户账号与角色实现计划.md"
---

# 02-用户账号与角色 PRD

## 1. 背景

平台需要邮箱登录和两级角色，为用户订阅日报、私有 RSS、管理员 API 提供身份基础。

## 2. 目标

实现 server 端邮箱账号体系、会话管理和 `user` / `admin` 两级角色。

## 3. 范围

- 邮箱注册、邮箱登录、退出登录、刷新 token。
- 密码哈希和密码重置预留。
- 用户 profile、状态、时区、日报发送时间。
- `user` 与 `admin` 角色字段。
- 管理员 API 鉴权中间件。

## 4. 非目标

- 不做复杂 RBAC。
- 不做多租户。
- 不做微信登录。
- 不做 Web 页面。

## 5. 用户故事

- 作为普通用户，我可以用邮箱注册登录并管理自己的订阅。
- 作为管理员，我可以访问管理员 API 管理来源、频道和任务。
- 作为系统，我可以拒绝普通用户访问管理员 API。

## 6. 数据与 API 边界

数据表：`users`、`sessions` 或 `refresh_tokens`。

API：`POST /api/v1/auth/register`、`POST /api/v1/auth/login`、`POST /api/v1/auth/refresh`、`POST /api/v1/auth/logout`、`GET /api/v1/me`。

## 7. 后台任务影响

无直接后台任务。邮件日报任务会读取用户邮箱、状态、时区和发送时间。

## 8. 配置影响

- `HOTKEY_AUTH_TOKEN_SECRET`
- `HOTKEY_AUTH_ACCESS_TOKEN_TTL`
- `HOTKEY_AUTH_REFRESH_TOKEN_TTL`

## 9. 错误与降级

- 邮箱重复返回结构化 `email_already_exists`。
- 密码错误返回统一认证失败，不泄漏账号存在性。
- token 过期返回 `401`。

## 10. 安全与合规

- 密码必须使用安全哈希。
- refresh token 只保存 hash。
- 管理员接口必须检查 `users.role=admin`。

## 11. 验收标准

- Given 新邮箱，When 注册，Then 创建 `user` 角色用户。
- Given 正确密码，When 登录，Then 返回 access token 和 refresh token。
- Given 普通用户 token，When 访问管理员 API，Then 返回 `403`。

## 12. TDD 验收标准

- Plan 必须先写失败测试，再写最小实现，再运行测试通过。
- 涉及 HTTP API 的任务必须包含 handler/contract 测试。
- 涉及数据库的任务必须包含 migration 和 repository 测试。
- 涉及 Redis 的任务必须包含队列、幂等、重试或降级测试。
- 涉及 DashScope 或 SMTP 的任务必须使用 fake/mock 测试，不依赖真实外部服务通过基础测试。

## 13. Harness 执行要求

每个 Linear issue 必须包含：

```text
1. Read PRD: docs/product/prd/02-用户账号与角色PRD.md
2. Read Plan: docs/plans/02-用户账号与角色实现计划.md
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

