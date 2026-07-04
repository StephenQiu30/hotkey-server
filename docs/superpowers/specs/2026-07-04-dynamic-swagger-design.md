# 动态 Swagger 接入收敛设计

## 背景

当前 `hotkey-server` 已经接入了 Swagger，但实现路径仍然混杂了静态产物、生成命令和额外配置。目标不是继续扩展一套“契约工程”，而是把 Swagger 收敛成常规后端项目的标准接入形态：像 Spring Boot 集成 Swagger 一样，接口文档由代码注释/注解驱动，通过访问后端接口动态获取文档信息和页面，不再把静态文档文件作为事实源。

本次设计只聚焦 `hotkey-server` 的运行时 Swagger 能力，不处理 `hotkey-web`、`hotkey-miniapp` 的下游联动方案，也不扩展新的 API 网关或文档平台能力。

## 目标

1. 通过访问后端接口动态获取 Swagger 文档数据和页面。
2. 文档来源统一为代码注释/注解驱动，而不是手写/静态维护的 JSON 文件。
3. 请求模型采用语义化 `XXXRequest` 命名。
4. 响应模型采用直观 `XXXResponse` 命名。
5. 注释量控制在“正常接入 Swagger”的水平，不为文档系统引入额外抽象层。

## 非目标

1. 不保留 `docs/openapi.json`、`docs/swagger.json`、`docs/swagger.yaml` 作为长期契约事实源。
2. 不保留 `make openapi`、`make swagger` 这类需要先生成静态文件的交互路径作为主流程。
3. 不自研运行时 OpenAPI 拼装器。
4. 不为 Swagger 建立额外的“契约层”“映射层”“文档 DTO 层”。
5. 不在本次设计中处理前端客户端代码生成策略。

## 推荐方案

### 方案 A：`swaggo/swag + gin-swagger`，运行时动态暴露，仅保留最小注释

这是推荐方案。

做法：

1. 使用 `swaggo/swag` 作为注释解析器。
2. 使用 `gin-swagger` 暴露 `/swagger/index.html`。
3. 使用 `/swagger/doc.json` 作为运行时文档接口。
4. 全局 API 元信息写在 `cmd/hotkey/main.go`。
5. 各接口说明写在 `internal/platform/http` 的命名 handler 上。
6. 请求/响应模型集中在 HTTP 层，命名统一为 `XXXRequest` / `XXXResponse`。

优点：

1. 最接近 Spring Boot 常规 Swagger 使用体验。
2. 完全符合 Gin 生态的标准路径，维护成本最低。
3. 不需要自己维护整份 OpenAPI 结构。
4. 注释天然和 handler、请求模型、响应模型同源。

缺点：

1. 需要保留最小量 Swagger 注释。
2. 需要确认运行时依赖是否仍会生成桥接代码；若必须存在桥接代码，只能将其视为运行时接入细节，而不是契约事实源。

### 方案 B：自定义基于 Gin 路由元信息的运行时文档组装

不推荐。

优点：

1. 表面上可以减少 Swagger 注释。

缺点：

1. 会重新发明半套 Swagger。
2. 请求体、响应体、状态码、鉴权、字段说明会迅速变得不完整。
3. 长期维护成本高，属于明显过度设计。

### 方案 C：保留静态产物，但运行时仍提供页面

不推荐。

优点：

1. 兼容离线文件消费。

缺点：

1. 与本次目标冲突。
2. 会重新把“仓库中文件”变成事实源，继续留下两套机制。

## 最终决策

采用方案 A。

核心原则只有一句：**Swagger 文档事实源是后端运行时接口，不是仓库里的静态文件。**

## 架构边界

Swagger 只属于 `hotkey-server` 的 HTTP 入口层，不单独形成“文档子系统”。

### 目录与职责

- `cmd/hotkey/main.go`
  - 放全局 Swagger 元信息注释
  - 不放业务接口实现
- `internal/platform/http`
  - 作为唯一 HTTP 接入层
  - 命名 handler 放在各模块文件中
- `internal/platform/http/request.go` 或按模块拆分的 request 文件
  - 存放 `XXXRequest`
- `internal/platform/http/response.go` 或按模块拆分的 response 文件
  - 存放 `XXXResponse` 与统一响应包装
- `/swagger/index.html`
  - Swagger UI 页面入口
- `/swagger/doc.json`
  - Swagger 文档 JSON 接口

### 明确删除/淘汰的路径

- `cmd/openapi`
- `BuildOpenAPISpec()` 一类静态 spec 拼装逻辑
- `GenerateOpenAPI()` 一类静态文件生成逻辑
- `docs/openapi.json`
- 围绕静态契约文件存在性的校验链

## 模型命名规范

### 请求模型

请求模型一律使用 `XXXRequest`。

示例：

- `RegisterRequest`
- `LoginRequest`
- `CreateMonitorRequest`
- `UpdateMonitorRequest`
- `ListMonitorPostsRequest`（只有在查询参数需要明确建模时才引入）

约束：

1. 不使用 `RegisterDTO`、`MonitorDTO` 这类弱语义命名。
2. 不把领域对象直接拿来承担 HTTP 入参。
3. 查询参数只有在语义复杂时才单独建模，简单 path/query 参数直接在 handler 注释中表达。

### 响应模型

响应模型统一使用 `XXXResponse`。

示例：

- `UserResponse`
- `LoginResponse`
- `MonitorResponse`
- `NotificationResponse`
- `MonitorListResponse`

约束：

1. 不混用 `VO`、`Envelope`、`DTO` 作为主命名。
2. 外层统一响应包装若继续存在，也应保持简单直观。
3. 不为了 Swagger 额外复制一份与 HTTP 返回一致的模型。

## Swagger 注释最小规范

只保留标准接入所需的最小注释集合：

- `@Summary`
- `@Tags`
- `@Accept`
- `@Produce`
- `@Param`
- `@Success`
- `@Failure`
- `@Security`
- `@Router`
- `@ID`

### 注释原则

1. 注释写在命名 handler 上，不写在匿名闭包上。
2. 只写对 Swagger 生成必要的信息。
3. 字段说明只有在字段语义不自解释时才补充。
4. 示例值只在前后端联调容易歧义的字段上添加。

### 明确不做

1. 不给每个字段写长段注释。
2. 不建立文档专用模型映射层。
3. 不因为 Swagger 把 handler 拆成大量无意义的小函数。
4. 不在 HTTP 层引入复杂的泛型包装体系来“美化”文档。

## 运行机制

### 目标行为

服务启动后：

1. 访问 `/swagger/index.html` 可看到 Swagger UI 页面。
2. 页面通过 `/swagger/doc.json` 获取接口文档。
3. 文档内容来源于代码注释/注解和 HTTP 层请求响应模型。

### 运行时事实源

运行时事实源是：

1. `cmd/hotkey/main.go` 的全局 Swagger 元信息。
2. `internal/platform/http` 中 handler 的 Swagger 注释。
3. HTTP 层 `XXXRequest` / `XXXResponse` 模型。

仓库中的静态 JSON/YAML 文件不是事实源，不参与当前设计定义。

## 迁移步骤

1. 清理旧链路
   - 删除 `cmd/openapi`
   - 删除静态 spec 组装逻辑
   - 删除对 `docs/openapi.json` 的依赖
2. 收口 HTTP 模型
   - 统一请求命名为 `XXXRequest`
   - 统一响应命名为 `XXXResponse`
3. 收口注释
   - 为命名 handler 添加最小 Swagger 注释
   - 为需要暴露的请求/响应模型补最小字段说明
4. 收口运行时入口
   - 只保留 `/swagger/index.html` 与 `/swagger/doc.json`
5. 收口验证
   - 从“静态文件存在”改为“运行时接口可访问且内容正确”

## 验证方案

### 开发验证

1. 启动服务。
2. 打开 `/swagger/index.html`。
3. 访问 `/swagger/doc.json`。
4. 确认关键接口路径存在：
   - `/api/v1/auth/register`
   - `/api/v1/auth/login`
   - `/api/v1/monitors`
5. 确认关键 `operationId` 存在。

### 自动化验证

建议保留三层验证：

1. 单元层
   - 验证 Swagger 文档中包含关键 path 与 `operationId`
2. Smoke 层
   - 服务启动后访问 `/swagger/index.html`
   - 服务启动后访问 `/swagger/doc.json`
3. 仓库总校验
   - 以“运行时页面与 JSON 可用”为准，而不是“某个静态文件已生成”

## 风险与约束

1. 如果所选 Swagger 方案在技术上必须生成桥接代码，允许保留最小运行时桥接文件，但不得再将其视为契约事实源。
2. 如果请求/响应模型继续和领域对象混杂，文档会再次变脏，因此 HTTP 模型边界必须明确。
3. 如果为减少注释而退回到手写运行时拼装，则会重新进入过度设计，不允许这样回退。

## 成功标准

满足以下条件即视为本次设计达标：

1. 后端文档通过接口动态获取，而不是依赖静态 JSON 文件。
2. 使用体验接近 Spring Boot 常规 Swagger 接入。
3. 注释量控制在正常接入水平，没有新增契约工程。
4. 请求命名统一为 `XXXRequest`。
5. 响应命名统一为 `XXXResponse`。
6. Swagger 页面和 JSON 都能在运行中的服务上直接访问。
