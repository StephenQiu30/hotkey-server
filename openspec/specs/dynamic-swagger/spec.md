# Dynamic Swagger Specification

## Purpose

将 `hotkey-server` 的 API 文档从静态文件（`openapi.json` / `swagger.json` / `swagger.yaml`）收敛为标准 Go Gin 生态的运行时 Swagger 方案。文档事实源为代码注释，通过 `swaggo/swag` + `gin-swagger` 运行时动态暴露。

## Requirements

### Requirement: Swagger 端点

系统 MUST 在非生产环境提供以下端点：

- `/swagger/index.html` — Swagger UI 页面
- `/swagger/doc.json` — 运行时 OpenAPI JSON 文档

生产环境 SHALL 默认不暴露 `/swagger/*` 端点，除非显式配置 `SWAGGER_ENABLED=true`。

### Requirement: 路由注册

1. `gin-swagger` middleware MUST 在 Gin 路由配置（`internal/platform/http/router.go`）中注册。
2. 路由注册 SHALL 仅在 `SWAGGER_ENABLED != false` 或非 `release` 模式下生效。

### Requirement: 全局元信息

1. Swagger 全局元信息（`@title`、`@version`、`@description`、`@host`、`@BasePath`、`@securityDefinitions.apikey`、`@license.name`）MUST 写在 `cmd/hotkey/main.go` 中。
2. 全局注释 SHALL 通过 `swag init` 生成 `docs/docs.go` 供 `gin-swagger` 使用。

### Requirement: 请求/响应模型命名

1. HTTP 请求模型 MUST 使用 `XXXRequest` 命名，不得使用 `DTO`、`VO`、`Params` 等变体。
2. HTTP 响应模型 MUST 使用 `XXXResponse` 命名，不得使用 `DTO`、`Envelope`、`VO` 等变体。
3. 领域对象 MUST NOT 直接作为 HTTP 入参模型使用。

### Requirement: Handler 注释

1. 各接口的 Swagger 注释 MUST 写在命名 handler 函数上，不得写在匿名闭包上。
2. POST/PATCH handler（接受请求体）MUST 包含 `@Accept json`。
3. 每个 exposed API handler MUST 包含 `@Summary`、`@Tags`、`@Produce`、`@Param`、`@Success`、`@Failure`、`@Router`、`@ID`。
4. GET handler（无请求体）SHOULD 省略 `@Accept`（语义上无 `consumes`）。

### Requirement: 运行时行为

1. 服务启动后 MUST 通过 `/swagger/doc.json` 返回完整的 API 文档。
2. 业务接口 MUST NOT 依赖 Swagger 中间件正常运行——中间件失败不阻塞业务路由。

### Requirement: 故障隔离

1. swaggo 注释解析失败时，SHALL 不影响业务接口调用。
2. `docs/docs.go` 过时时，SHALL 通过构建流水线中的 `make swagger`（`swag init`）检测差异并重新生成。
3. 运行时 smoke 验证 SHALL 确认 `/swagger/index.html` 和 `/swagger/doc.json` 可达。

### Requirement: 构建流水线

- `make swagger` target MUST 执行 `swag init -g cmd/hotkey/main.go -o docs --parseInternal --ot go`。
- `make swagger` SHALL 作为 CI 流水线的一部分（`ci` target 中的前置步骤）在 `lint`/`build`/`test` 之前执行。

## Verification

1. 单元测试 MUST 验证 `/swagger/doc.json` 包含关键 path 和 `operationId`。
2. Smoke 测试 MUST 验证 `/swagger/index.html` 返回 200。
3. 回滚后 MUST 验证 `make swagger` 可重新生成有效 `docs/docs.go`。
4. 全局元信息注入后 MUST 验证 `/swagger/doc.json` 包含 `info.title`、`info.version`、`host`、`basePath` 和 `securityDefinitions`。
