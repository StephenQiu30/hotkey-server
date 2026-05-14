# Proposal: GitHub OAuth 登录认证（单用户 SaaS）

## Why

当前 SaaS 前端与后端接口已具备完整业务闭环，但缺少登录鉴权。用户要求以 GitHub 登录作为访问控制入口，需要在不影响现有采集/检测链路的前提下补齐完整认证链路。

## Scope

- 后端提供 GitHub OAuth 登录/回调与会话签发能力。
- 后端增加 `users` 持久化与会话鉴权依赖，受保护 API 默认要求携带 `Authorization: Bearer <token>`。
- 前端增加登录页、GitHub 登录触发、OAuth 回调落地、登录态存储与 `/app` 路由守卫。
- 使用 openspec 的 `tasks.md` 跟踪可交付项，形成可验收变更。

## Non-goals (当前版本)

- 不引入多用户租户和 RBAC；当前默认单用户/单组织访问模式。
- 不实现 token 刷新链路与会话撤销中心。
- 不改造现有数据库 schema 版本体系（继续沿用 `sql/001_init_schema.sql` 直接管理）。
