---
layer: PRD
doc_no: "08"
audience:
  - PM
  - Tech-Lead
  - Dev
  - QA
  - Ops
feature_area: backend-security
purpose: "定义企业级 P0 安全增强中的 JWT 账号体系下 RBAC 最小可运行能力（A8）。"
canonical_path: "docs/product/prd/08-安全增强-RBAC与权限治理PRD.md"
status: draft
version: "0.1.0"
owner: "StephenQiu30"
inputs:
  - "docs/engineering/验收标准.md（A8）"
  - "server/app/core/security.py"
  - "server/app/api/routes/*"
outputs:
  - "最小 RBAC 权限模型与接口依赖"
  - "角色字段与权限错误返回一致性"
downstream:
  - "docs/plans/15-安全增强与RBAC计划.md"
  - "docs/engineering/验收标准.md"
---

# 安全增强：JWT 与 RBAC 治理 PRD（A8）

## 1. 背景

后端已具备会话 token 与鉴权网关，但尚未形成稳定的权限分层。为了满足企业级 P0 的安全加固，需补齐“角色 + 权限 + 路由控制”的最小闭环，并兼容现有 JWT 登录链路。

## 2. 目标（SMART）

- 在不破坏现有认证方式的前提下，支持最小角色模型：
  - `admin`：可执行管理类写操作（关键词、来源、设置、任务管理）；
  - `viewer`：仅可读关键资源。
- 将关键写接口接入权限依赖，形成统一报错行为（401 未登录，403 权限不足）。
- 与已有限流、审计中间件协同运行，不引入外部鉴权网关。

### 验收标准

- 注册/登录后 token 仍能用于正常鉴权访问。
- `viewer` 在只读场景可查询热点/列表，在受限场景返回 `403`。
- `admin` 可执行 `keyword.source.setting` 等管理路由。
- OpenAPI/Contract 中角色字段或鉴权策略可见（不要求新增 UI）。

## 3. 非目标

- 不引入 OAuth 作用域、API Key 管理、组织/租户隔离。
- 不做复杂审计台账持久化，仅保留内置日志与现有 request metrics。
- 不新增网关级策略引擎。

## 4. 功能定义

- 用户模型新增 `role` 字段（不要求历史迁移脚本，schema SQL 已同步）；
- 认证上下文新增 `require_permission(permission)` 依赖；
- 路由分层：
  - `keyword/source/setting/check-run/report` 的写链路按配置要求绑定对应权限；
  - 读链路保持可访问。
- 错误策略统一为 `HTTP 403 + "Insufficient permissions."`。
- 预留 `/api/auth/me` 对角色返回（用于前端或管理动作判断）。

## 5. 风险与边界

- 现有数据库若有历史账号，无 role 记录时需兼容处理；
- 未来新增路由需同步声明权限，不然可能默认放开。
- 在不引入前端权限页的场景，角色分配默认依赖服务端初始化配置。

## 6. 交付与验收

- 在 `server/app/models/user.py` 与 `sql/001_init_schema.sql` 增加 `role`；
- 在 `server/app/core/security.py` 实现 `Permission` 依赖；
- 在关键路由补齐权限依赖；
- 增加 `tests/test_mvp_services.py` 的 RBAC 权限测试。

## 7. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-24 | StephenQiu30 | 0.1.0 | 新建企业级 A8 安全补齐 PRD |
