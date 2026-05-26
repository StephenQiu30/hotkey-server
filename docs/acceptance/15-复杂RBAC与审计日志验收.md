# 15-复杂 RBAC 与审计日志验收

## 验收范围

- 租户内角色授权：`POST /api/v1/admin/tenants/{id}/roles`。
- 租户内权限判定：`POST /api/v1/admin/tenants/{id}/authorize`。
- 租户内审计日志：`GET /api/v1/admin/tenants/{id}/audit-logs`。
- 角色边界：`owner` 可管理角色、关键词、来源和日报查看；`admin` 可管理关键词、来源和日报查看；`viewer` 只能查看日报。
- 审计边界：角色授权和关键配置变更需要按租户隔离记录审计事件。

## 非目标

- 不在本任务实现租户级配置扩展，该范围属于 #86。
- 不引入外部 IAM、OAuth、SSO 或细粒度策略语言。
- 不实现真实数据库审计表，当前先用进程内实现锁定契约。

## 验收命令

```bash
go test ./...
git diff --check
find docs/product/prd docs/plans -maxdepth 1 -type f | sort -V
```

## 验收结果

- RBAC 服务测试覆盖租户内角色权限边界和跨租户隔离。
- 审计测试覆盖关键配置变更审计和租户级审计日志过滤。
- HTTP 测试覆盖角色授权、权限判定和审计日志查询。
- OpenAPI 测试覆盖 RBAC 与审计日志契约路径和状态码。
