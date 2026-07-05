## Why

当前 `hotkey-server` 已接入 Swagger，但实现路径混杂了静态产物（`docs/openapi.json`、`docs/swagger.json`、`docs/swagger.yaml`）、生成命令（`make openapi`、`make swagger`）和额外配置。文档事实源是仓库中的静态文件，而非运行时接口。这与 Go Gin 生态标准用法不一致，导致：

1. 每次修改接口需手动重新生成静态文件，容易遗漏。
2. 静态产物与代码注释可能漂移不一致。
3. 新开发者需要理解额外的"契约工程"流程才能参与。

需要将 Swagger 收敛为常规后端项目的标准接入形态，使文档由代码注释/注解驱动，通过访问后端接口动态获取。

本 change 对应 `docs/superpowers/specs/2026-07-04-dynamic-swagger-design.md`（SP-SDD-001）。

## What Changes

- 安装 swaggo CLI 依赖并配置 `Makefile` 构建流水线
- 在 `cmd/hotkey/main.go` 添加全局 Swagger 元信息注释
- 配置 gin-swagger 路由，在非生产环境注册 `/swagger/*` 路由
- 在各 handler 上添加最小 Swagger 注释（`@Summary`、`@Tags`、`@Param`、`@Success`、`@Failure`、`@Router` 等）
- 统一 HTTP 请求模型命名为 `XXXRequest`
- 统一 HTTP 响应模型命名为 `XXXResponse`
- 删除旧静态 OpenAPI 链路（`cmd/openapi`、`BuildOpenAPISpec()`、`docs/openapi.json` 等）
- 更新 `scripts/validate-repository.sh` 移除对静态文件的依赖检查
- 新增运行时 smoke 验证确认 `/swagger/index.html` 和 `/swagger/doc.json` 可访问

## Capabilities

### New Capabilities
- `dynamic-swagger`: 运行时 Swagger 文档，通过 `/swagger/index.html` 和 `/swagger/doc.json` 获取，文档事实源为代码注释而非静态文件

### Modified Capabilities
- `api-documentation`: 从静态文件契约切换为运行时接口驱动
- `repository-validation`: `scripts/validate-repository.sh` 移除对 `docs/openapi.json` 静态文件的依赖检查

### Removed Capabilities
- `static-openapi`: 废弃 `cmd/openapi`、`BuildOpenAPISpec()`、`GenerateOpenAPI()` 等静态拼装逻辑
- `make openapi` / `make swagger` 静态生成 targets（迁移到 `make swagger` 作为构建流水线步骤）

## Impact

- 新增 `github.com/swaggo/gin-swagger`、`github.com/swaggo/files` 依赖
- 新增 `docs/docs.go`（swaggo 自动生成产物，不手工维护）
- 修改 `cmd/hotkey/main.go`（添加全局 Swagger 注释和路由注册）
- 修改 `internal/platform/http/` 各 handler（添加最小 Swagger 注释）
- 修改 `internal/platform/http/request.go` 或相关文件（统一为 `XXXRequest` 命名）
- 修改 `internal/platform/http/response.go` 或相关文件（统一为 `XXXResponse` 命名）
- 删除 `cmd/openapi/` 目录
- 删除 `BuildOpenAPISpec()` 等静态 spec 拼装逻辑
- 删除 `docs/openapi.json`、`docs/swagger.json`、`docs/swagger.yaml`
- 修改 `Makefile`（新增 `swagger` target、清理旧 `openapi` target）
- 修改 `scripts/validate-repository.sh`（移除对静态 OpenAPI 文件的检查）
