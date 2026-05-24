# 15-安全增强与RBAC计划

## 目标

补齐企业级 A8：JWT 已有基础上补齐最小 RBAC 与权限错误边界控制。

## 范围与不变项

- 范围：
  - 用户角色字段；
  - 权限依赖函数；
  - 关键写操作添加鉴权；
  - 失败场景 `403` 与日志一致性。
- 不变项：
  - 现有 `/api/auth` 登录、注册、token 发放行为不变；
  - 既有限流和审计中间件不变。

## 任务拆解

- **P0-A8-1：模型与 schema**
  - 在 `server/app/models/user.py` 增加 `role`；
  - 在 `server/app/core/settings.py` 可选补充默认角色配置；
  - 在 `server/app/schemas/auth.py` 暴露 `role`。

- **P0-A8-2：权限依赖**
  - 在 `server/app/core/security.py` 实现 `require_permission(permission)`；
  - 权限映射：
    - `keyword.manage`
    - `source.manage`
    - `report.manage`
    - `settings.manage`
    - `task.manage`
    - `readOnly`（默认只读）

- **P0-A8-3：路由接入**
  - 给 `keywords/sources/settings/check-runs/reports` 的写操作接入权限依赖；
  - 保留 `GET /api/hotspots`、`GET /api/hotspots/{id}` 的读取能力。

- **P0-A8-4：测试**
  - 在 `tests/test_mvp_services.py` 增加：
    - viewer 用户访问受限写路由返回 `403`；
    - admin 或默认角色可访问写路由。

## 依赖与顺序

- 先补权限模型与依赖，再接入路由，再补对应测试。
- 遵循 `test:` → `impl:` → `refactor:` 提交流程。

## 验收

- 测试通过，错误码符合 401/403；
- OpenAPI 文档包含新增 role 字段（`UserRead`）；
- `docs/engineering/验收标准.md` 的 A8 更新为“已完成/待复核”。
