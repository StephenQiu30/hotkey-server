# 16-租户级配置与管理员后台 API 扩展验收

## 验收范围

- 平台管理员可跨租户治理：`GET /api/v1/admin/tenants`。
- 租户管理员可管理本租户关键词：
  - `GET /api/v1/admin/tenants/{id}/keywords`
  - `POST /api/v1/admin/tenants/{id}/keywords`
- 租户管理员可管理本租户来源：
  - `GET /api/v1/admin/tenants/{id}/sources`
  - `POST /api/v1/admin/tenants/{id}/sources`
  - `PATCH /api/v1/admin/tenants/{id}/sources/{sourceId}`
- 租户日报沿用 `GET /api/v1/tenants/{id}/reports/daily`。
- OpenAPI 导出包含租户管理员后台扩展契约。

## 非目标

- 不在本任务实现真实平台超级管理员认证。
- 不引入前端管理后台页面。
- 不扩展 P2 计费、多服务拆分或复杂消息队列。

## 验收命令

```bash
go test ./...
git diff --check
find docs/product/prd docs/plans -maxdepth 1 -type f | sort -V
```

## 验收结果

- HTTP 测试覆盖租户级关键词创建和隔离查询。
- HTTP 测试覆盖租户级来源创建、更新和隔离查询。
- HTTP 测试覆盖平台管理员列出租户。
- OpenAPI 测试覆盖租户管理员后台扩展路径和状态码。
