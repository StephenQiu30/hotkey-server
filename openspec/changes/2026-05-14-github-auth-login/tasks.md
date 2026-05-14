# OpenSpec Tasks: GitHub Login & Auth Gate

- [x] Add backend auth data model and token/security utilities.
  - [x] Add `apps/api/app/models/user.py`。
  - [x] Add `apps/api/app/core/security.py`（签发、校验、自定义 token，`get_current_user`）。
  - [x] Add `users` 表到 `sql/001_init_schema.sql` 并补索引。
- [x] Build GitHub auth API routes and integrate with router security policy.
  - [x] Add `apps/api/app/api/routes/auth.py`（`/api/auth/github/login`、`/api/auth/github/callback`、`/api/auth/me`）。
  - [x] Wire `auth_router` and protect existing routes with `get_current_user` dependency.
  - [x] Expose env config in `apps/api/app/core/settings.py` and `infra/env/.env.example`（GitHub + JWT + web base）。
- [x] Implement frontend auth flow and guards.
  - [x] Extend `apps/web/src/lib/api.ts`：token 管理 + 登录 API + `/api/auth/me` 调用能力。
  - [x] Add login page `/login` and callback page `/auth/github/callback`.
  - [x] Add auth gate in `apps/web/src/components/AppShell.tsx`（未登录重定向到登录）。
- [x] Smoke verification.
  - [ ] 前端登录主链路通路验证（点击登录 -> GitHub 授权 -> 回调存 token -> 进入 `/app`，待人工手测）。
  - [x] 关键接口 401 行为回归（无 token 访问 `/api/keywords`）。
  - [x] 文档化变更与任务归档。
