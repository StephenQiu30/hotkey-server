# Tasks: 动态 Swagger 接入收敛

## Task 1: 准备 swaggo 环境与依赖

- [ ] 1.1 安装 swaggo CLI：`go install github.com/swaggo/swag/cmd/swag@latest`
- [ ] 1.2 添加 Go 依赖：`go get github.com/swaggo/gin-swagger github.com/swaggo/files`
- [ ] 1.3 执行 `swag init` 验证可正常生成 `docs/docs.go`
- [ ] 1.4 将 `swag init` 加入 `Makefile`（新增 `swagger` target）
- [ ] 1.5 更新 `.PHONY` 列表

## Task 2: 添加全局 Swagger 注释

- [ ] 2.1 在 `cmd/hotkey/main.go` 添加 `@title`、`@version`、`@description`、`@host`、`@BasePath`、`@securityDefinitions.apikey` 全局注释
- [ ] 2.2 在 `cmd/hotkey/main.go` 配置 gin-swagger 路由（`r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))`）
- [ ] 2.3 通过环境变量 `SWAGGER_ENABLED` 控制路由注册：默认 `release` 模式下不注册

## Task 3: 统一 HTTP 请求模型命名

- [ ] 3.1 枚举 `internal/platform/http/` 中所有当前请求模型命名
- [ ] 3.2 将非 `XXXRequest` 命名的模型统一重命名为 `XXXRequest`
- [ ] 3.3 更新所有引用重命名模型的 handler 和测试文件
- [ ] 3.4 确保领域对象不直接作为 HTTP 入参

## Task 4: 统一 HTTP 响应模型命名

- [ ] 4.1 枚举 `internal/platform/http/` 中所有当前响应模型命名
- [ ] 4.2 将非 `XXXResponse` 命名的模型统一重命名为 `XXXResponse`
- [ ] 4.3 更新所有引用重命名模型的 handler 和测试文件

## Task 5: 添加 handler Swagger 注释

- [ ] 5.1 为每个认证相关 handler 添加最小 Swagger 注释（`@Summary`、`@Tags`、`@Accept`、`@Produce`、`@Param`、`@Success`、`@Failure`、`@Router`、`@ID`）
- [ ] 5.2 为每个监控相关 handler 添加最小 Swagger 注释
- [ ] 5.3 为每个其他 exposed handler 添加最小 Swagger 注释
- [ ] 5.4 重新执行 `swag init` 验证注释可被正确解析

## Task 6: 清理旧静态 OpenAPI 链路

- [ ] 6.1 删除 `cmd/openapi` 目录
- [ ] 6.2 删除 `BuildOpenAPISpec()` 等静态 spec 拼装逻辑
- [ ] 6.3 删除 `GenerateOpenAPI()` 等静态文件生成逻辑
- [ ] 6.4 删除 `docs/openapi.json`
- [ ] 6.5 更新 `scripts/validate-repository.sh`：移除对 `docs/openapi.json` 的依赖检查
- [ ] 6.6 清理围绕静态契约文件的测试（`internal/platform/http/openapi_coverage_test.go` 等）
- [ ] 6.7 **注意**：上述删除操作必须在独立 commit 中完成，支持独立回滚

## Task 7: 运行时验证

- [ ] 7.1 启动服务后访问 `/swagger/index.html` 确认返回 Swagger UI 页面
- [ ] 7.2 访问 `/swagger/doc.json` 确认返回完整 OpenAPI JSON
- [ ] 7.3 确认关键接口路径存在：`/api/v1/auth/register`、`/api/v1/auth/login`、`/api/v1/monitors`
- [ ] 7.4 确认关键 `operationId` 存在
- [ ] 7.5 单元测试：验证 `/swagger/doc.json` 包含关键 path 与 `operationId`
- [ ] 7.6 Smoke 测试：验证 `/swagger/index.html` 返回 200
- [ ] 7.7 执行 `make test` 和 `make lint` 确保无回归

## Task 8: 文档与 OpenSpec 同步

- [ ] 8.1 确保 `docs/superpowers/specs/2026-07-04-dynamic-swagger-design.md` 保持为最新的事实源
- [ ] 8.2 归档本 change：同步 `openspec/specs/`
- [ ] 8.3 更新 `openspec/changes/` 中的 change 状态
