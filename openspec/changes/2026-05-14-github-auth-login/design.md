# Design: GitHub Login and Authorization Gate

## Backend Design

- 新增 `apps/api/app/models/user.py` 与 `sql/001_init_schema.sql` 中 `users` 表，用于保存 GitHub 用户主索引与基本资料。
- 新增 `apps/api/app/core/security.py` 实现：
  - HMAC 签名会话 token（轻量自签）；
  - OAuth state token 与校验；
  - `get_current_user` 依赖。
- 新增 `apps/api/app/api/routes/auth.py`：
  - `GET /api/auth/github/login`：返回 `authorization_url`；
  - `GET /api/auth/github/callback`：code 换 token、抓取 GitHub 用户、写库、颁发会话 token，并跳转到前端回调页；
  - `GET /api/auth/me`：返回当前登录用户。
- 运行时路由保护：除 `/api/health`、`/api/auth/*` 外，其余业务 API 默认挂载鉴权依赖。

## Frontend Design

- `apps/web/src/lib/api.ts` 增加 token 管理（读取、设置、清理）和 `getCurrentUser`/`getAuthLoginUrl`。
- `apps/web/src/components/AppShell.tsx` 中做 `/app` 入口鉴权；未登录自动跳转 `/login`。
- 新增页面：
  - `/login`：提供 GitHub 登录按钮；
  - `/auth/github/callback`：读取 `token`，落库 localStorage，跳转工作台。

## OpenSpec Plan

- 任务以 openspec change 形式跟踪，按“低耦合序列”推进：
  - 先接口层，再权限层，再页面层，最后联调与验收归档。
