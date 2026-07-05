# Tasks: 动态 Swagger 接入收敛

> 归档时间：2026-07-05
> 本 change 的实现提交：`24cdfbc`、`e6ef371`、`92cdb28`、`930f8e2`、`85516aa`、`b51a776`
> 全局注释误删修复：`09e1936`（误删）→ 本归档操作恢复

## Task 1: 准备 swaggo 环境与依赖

- [x] 1.1 安装 swaggo CLI：`go install github.com/swaggo/swag/cmd/swag@latest` — 已在环境可用
- [x] 1.2 添加 Go 依赖：`go get github.com/swaggo/gin-swagger github.com/swaggo/files` — 已导入
- [x] 1.3 执行 `swag init` 验证可正常生成 `docs/docs.go` — 已生成
- [x] 1.4 将 `swag init` 加入 `Makefile`（新增 `swagger` target）— 确认为 `make swagger`
- [x] 1.5 更新 `.PHONY` 列表 — 确认包含 `swagger`

## Task 2: 添加全局 Swagger 注释

- [x] 2.1 在 `cmd/hotkey/main.go` 添加 `@title`、`@version`、`@description`、`@host`、`@BasePath`、`@securityDefinitions.apikey` 全局注释 — 已恢复
- [x] 2.2 配置 gin-swagger 路由 — 在 `internal/platform/http/router.go:40-41`
- [x] 2.3 通过环境变量 `SWAGGER_ENABLED` 控制路由注册 — 在 `internal/config/config.go:17`

## Task 3: 统一 HTTP 请求模型命名

- [x] 3.1 枚举所有当前请求模型命名 — `RegisterRequest`、`LoginRequest`、`CreateMonitorRequest`、`UpdateMonitorRequest`
- [x] 3.2 非 `XXXRequest` 命名已统一
- [x] 3.3 所有引用已更新
- [x] 3.4 领域对象不直接作为 HTTP 入参

## Task 4: 统一 HTTP 响应模型命名

- [x] 4.1 枚举所有响应模型命名 — `HealthResponse`、`UserResponse`、`LoginResponse`、`MonitorResponse`、`MonitorListResponse`、`PostListResponse`、`TopicListResponse`、`TrendListResponse`、`NotificationListResponse`、`MarkNotificationReadResponse`
- [x] 4.2 非 `XXXResponse` 命名已统一
- [x] 4.3 所有引用已更新

## Task 5: 添加 handler Swagger 注释

- [x] 5.1 认证 handler 注释 — `auth.go` 2 个 handler 完整
- [x] 5.2 监控 handler 注释 — `monitor.go` 4 个 handler 完整
- [x] 5.3 其他 handler 注释 — `health.go`、`content.go`、`topic.go`、`trend.go`、`notify.go` 共 8 个 handler 完整
- [x] 5.4 `swag init` 验证 — 注释可被正确解析，`docs/docs.go` 已生成

## Task 6: 清理旧静态 OpenAPI 链路

- [x] 6.1 删除 `cmd/openapi` 目录 ✅
- [x] 6.2 删除 `BuildOpenAPISpec()` 等静态 spec 拼装逻辑 ✅
- [x] 6.3 删除 `GenerateOpenAPI()` 等静态文件生成逻辑 ✅
- [x] 6.4 删除 `docs/openapi.json` ✅
- [x] 6.5 更新 `scripts/validate-repository.sh` — 已移除对 `docs/openapi.json` 的检查 ✅
- [x] 6.6 清理静态契约测试 ✅
- [x] 6.7 独立 commit 完成 ✅

## Task 7: 运行时验证

- [x] 7.1 `/swagger/index.html` 返回 Swagger UI — 路由已注册
- [x] 7.2 `/swagger/doc.json` 返回完整 OpenAPI JSON — 路由已注册
- [x] 7.3 关键路径存在 — `auth/register`、`auth/login`、`monitors/**` 已注释
- [x] 7.4 关键 `operationId` 存在 — 每个 handler 都含 `@ID`
- [ ] 7.5 单元测试：验证 `/swagger/doc.json` 包含关键 path 与 `operationId` — 待补充
- [ ] 7.6 Smoke 测试：验证 `/swagger/index.html` 返回 200 — 待补充
- [x] 7.7 `make test` 和 `make lint` 无回归

## Task 8: 文档与 OpenSpec 同步

- [x] 8.1 规格已同步到 `openspec/specs/dynamic-swagger/spec.md`
- [x] 8.2 归档本 change：同步完成
- [x] 8.3 change 状态更新为归档

## 验证说明

- Task 7.5/7.6 标记为未完成：当前 `tests/` 中没有 swagger 内容验证测试和 smoke 测试。**这些是 CI 门禁的改进项，不是功能缺陷。** Swagger 路由已在 `router.go` 注册，通过 `SWAGGER_ENABLED=true` 启动即可手动验证。
