# 12-管理员 API 契约验收

## 验收范围

- 管理员关键词启停：`GET /api/v1/admin/keywords`、`POST /api/v1/admin/keywords`、`PATCH /api/v1/admin/keywords/{id}`。
- 管理员来源启停：`GET /api/v1/admin/sources`、`PATCH /api/v1/admin/sources/{id}`。
- 管理员任务运行查询：`GET /api/v1/admin/task-runs`，支持 `status=failed` 过滤失败记录。
- 管理员日报触发：`POST /api/v1/admin/reports/daily`，返回 `202`、日报结果和任务运行记录。
- OpenAPI 导出包含管理员 API 契约，并保留 `BearerAuth` 与结构化错误响应。

## 验收命令

```bash
go test ./...
git diff --check
find docs/product/prd docs/plans -maxdepth 1 -type f | sort -V
```

## 验收结果

- 管理员 API 服务层测试覆盖任务成功、失败记录和失败过滤。
- HTTP 测试覆盖管理员手动触发平台日报，以及任务运行记录查询。
- OpenAPI 测试覆盖管理员关键词、来源、任务、日报和事件管理契约。
