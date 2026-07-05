# Specs: 动态 Swagger 接入收敛

## 接口契约

### Swagger 端点

1. `/swagger/index.html` MUST 在非生产环境返回 Swagger UI 页面。
2. `/swagger/doc.json` MUST 返回完整的运行时 OpenAPI JSON 文档。
3. 生产环境 SHALL 默认不暴露 `/swagger/*` 端点，除非显式配置 `SWAGGER_ENABLED=true`。

### 路由注册

1. `gin-swagger` middleware MUST 在 `cmd/hotkey/main.go` 中注册。
2. 路由注册 SHALL 仅在 `SWAGGER_ENABLED != false` 或非 `release` 模式下生效。

### 请求/响应模型

1. HTTP 请求模型 MUST 使用 `XXXRequest` 命名，不得使用 `DTO`、`VO`、`Params` 等变体。
2. HTTP 响应模型 MUST 使用 `XXXResponse` 命名，不得使用 `DTO`、`Envelope`、`VO` 等变体。
3. 领域对象 MUST NOT 直接作为 HTTP 入参模型使用。

## 注释规范

1. 全局 Swagger 元信息 MUST 写在 `cmd/hotkey/main.go`。
2. 各接口的 Swagger 注释 MUST 写在命名 handler 函数上，不得写在匿名闭包上。
3. 每个 exposed API handler MUST 包含 `@Summary`、`@Tags`、`@Accept`、`@Produce`、`@Param`、`@Success`、`@Failure`、`@Router`、`@ID`。
4. 字段注释 SHOULD 仅在字段语义不自解释时补充。

## 运行时行为

1. 服务启动后 MUST 通过 `/swagger/doc.json` 返回完整的 API 文档。
2. 业务接口 MUST NOT 依赖 Swagger 中间件正常运行——中间件失败不阻塞业务路由。

## 失败场景

- swaggo 注释解析失败时，SHALL 不影响业务接口调用。
- `docs/docs.go` 过时时，SHALL 通过构建流水线中的 `swag init` 检测差异并重新生成。
- 运行时 smoke 验证 SHALL 确认 `/swagger/index.html` 和 `/swagger/doc.json` 可达。

## 验证

1. 单元测试 MUST 验证 `/swagger/doc.json` 包含关键 path 和 `operationId`。
2. Smoke 测试 MUST 验证 `/swagger/index.html` 返回 200。
3. 回滚后 MUST 验证 `make openapi` 可重新生成有效 `docs/openapi.json`。
