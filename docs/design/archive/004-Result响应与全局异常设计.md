---
layer: Design
doc_no: "004"
audience: Dev, QA
feature_area: API响应与异常
purpose: 定义所有JSON接口统一使用的Result响应和全局异常转换规则
canonical_path: docs/design/archive/004-Result响应与全局异常设计.md
status: accepted
version: v1.2
owner: HotKey Server Team
inputs:
  - AGENTS.md
  - docs/design/archive/002-后端单体架构设计.md
outputs:
  - Result响应契约
  - 业务错误码规则
  - 全局异常处理流程
triggers:
  - 修改API响应字段、错误码或异常映射
  - 新增业务错误领域
downstream:
  - OpenAPI
  - 所有Controller与Service
  - docs/prd/archive/003-HTTP契约安全与可观测基础.md
  - docs/plans/archive/003-HTTP契约安全与可观测基础计划.md
  - docs/acceptance/archive/003-HTTP契约安全与可观测基础验收.md
---

# Result 响应与全局异常设计

本文定义统一 API 响应。所有 JSON 接口只返回 `code`、`message`、`data`，并通过全局错误处理器将领域错误、校验错误和 panic 转换为同一结构。

## Result 契约

```go
type Result[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}
```

成功响应：

```json
{
  "code": 0,
  "message": "success",
  "data": {
    "id": 1001,
    "name": "OpenAI"
  }
}
```

失败响应：

```json
{
  "code": 30001,
  "message": "监控任务不存在",
  "data": null
}
```

`data` 字段始终存在。无返回数据和所有错误响应都使用 `null`。

## 分页数据

```go
type Page[T any] struct {
	Items    []T   `json:"items"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}
```

分页对象作为 Result 的 `data` 返回，不增加顶层字段。

## HTTP 与业务码

HTTP 状态保留协议语义，业务 `code` 用于客户端逻辑和错误定位。禁止所有错误统一返回 HTTP 200。

| HTTP | 场景 |
|---|---|
| 200 | 查询、更新、删除成功 |
| 201 | 创建成功 |
| 400 | 参数格式或校验失败 |
| 401 | 未登录或会话无效 |
| 403 | 无访问权限 |
| 404 | 资源不存在 |
| 409 | 重复创建或状态冲突 |
| 429 | 用户或来源限流 |
| 500 | 未预期内部错误 |
| 502 | 外部平台异常 |
| 503 | 数据库、来源或AI服务不可用 |
| 504 | 外部调用超时 |

业务码分段：

| 范围 | 领域 |
|---|---|
| 0 | 成功 |
| 10000-19999 | 通用请求和校验 |
| 20000-29999 | 用户、认证和权限 |
| 30000-39999 | 监控和查询计划 |
| 40000-49999 | 来源和采集 |
| 50000-59999 | 内容、匹配和去重 |
| 60000-69999 | 事件、热度和趋势 |
| 70000-79999 | AI、Embedding 和摘要 |
| 80000-89999 | 调度、通知和运行 |
| 90000-99999 | 系统和未知异常 |

已发布业务码不得改变语义或重复使用。

## AppError

```go
type AppError struct {
	Code       int
	HTTPStatus int
	Message    string
	Retryable  bool
	Cause      error
}
```

各模块在自己的 `errors.go` 定义领域错误。Controller 不创建数据库错误映射，也不拼装错误 JSON。

Repository 将数据库错误转换为领域可理解的错误，Service 可以补充业务语义。未识别错误统一转换为 `90000`。

## Handler 包装

```go
type HandlerFunc func(*gin.Context) error

func Wrap(handler HandlerFunc) gin.HandlerFunc
```

Controller 成功时使用 Result 工具返回，失败时直接返回 error：

```go
func CreateMonitor(c *gin.Context) error {
	monitor, err := service.Create(c.Request.Context(), request)
	if err != nil {
		return err
	}
	result.Created(c, monitor)
	return nil
}
```

业务 Controller 禁止直接调用 `c.JSON`。架构校验脚本需要阻止绕过 Result 的 JSON 输出。

## Result 工具

Result 包只提供必要方法：

```go
func OK[T any](c *gin.Context, data T)
func Created[T any](c *gin.Context, data T)
func Empty(c *gin.Context)
func PageOK[T any](c *gin.Context, page Page[T])
func Fail(c *gin.Context, status, code int, message string)
```

`Empty` 返回 HTTP 200 和 `data: null`。MVP 不使用 204，以保证所有 JSON 操作响应都具有 Result 结构。

## 中间件顺序

```text
RequestID
-> TraceContext
-> AccessLog
-> Recovery
-> CORS
-> RequestContextTimeout
-> Authentication
-> Authorization
-> Handler
-> GlobalErrorHandler
```

`X-Request-ID` 写入响应头、日志和 trace 属性，不加入 Result。`TraceContext` 使用 W3C Trace Context 提取和传播上下文；AccessLog 在请求完成后记录最终 status、route、耗时和 request ID。

`Authentication` 与 `Authorization` 是固定扩展位：PLAN-003 只提供无 token 解析的中性 passthrough 及 `401`/`403` 的 AppError 映射，PLAN-004 才注入身份、会话和权限规则。请求超时为 request context 设置 deadline；若 Handler 返回 deadline 错误，GlobalErrorHandler 输出安全的 504 Result。

`GlobalErrorHandler` 的唯一实现入口是 `Wrap(HandlerFunc)`：Handler 成功时只能调用 Result 工具，失败时返回 error；`Wrap` 调用错误映射和 `Fail`。Controller 或业务 transport 不得直接调用 Gin JSON 方法，也不得自行写错误响应。

Recovery 捕获 panic 后：

- 返回 HTTP 500 和业务码 `90000`
- 记录请求 ID、模块和内部堆栈
- 不返回堆栈、SQL、路径、密钥或 Cause
- 不允许单个请求 panic 导致进程退出

## 错误转换

| 内部错误 | 对外结果 |
|---|---|
| 参数绑定或校验失败 | HTTP 400 和通用参数错误码 |
| Record not found | 模块 NOT_FOUND 错误 |
| 唯一约束冲突 | HTTP 409 |
| Context deadline | HTTP 504 |
| 来源限流 | HTTP 429 或后台延迟重试 |
| 来源 5xx | HTTP 502 或后台可重试错误 |
| LLM 暂时不可用 | HTTP 503 或使用旧摘要降级 |
| 未识别错误 | HTTP 500 和 `90000` |

参数错误只通过 `message` 返回必要说明，不增加 `details` 字段。

## 日志和安全

- 4xx 记录为 info 或 warn
- 5xx 记录为 error 并保留内部 Cause
- 日志必须包含 request ID、路由、模块和耗时
- 日志不得记录密码、Token、API Key 或完整 Authorization Header
- 外部平台原始错误只进入受控内部日志

## OpenAPI

所有 `/api/v1` JSON 接口必须在 OpenAPI 中声明 Result 响应。分页、详情、列表和空数据通过不同 `data` 类型表达，不创建不同顶层响应格式。

`/healthz` 与 `/readyz` 是非 `/api/v1` 的进程运维探针：它们使用同一安全 Result 与错误脱敏规则，但明确排除在生成的 OpenAPI 外。不得仅因排除 OpenAPI 而改用裸文本、`c.JSON` 或暴露内部依赖细节。

平台在没有业务资源前，使用唯一的 `GET /api/v1/capabilities` 作为非业务的公开契约对象，返回 Result 内的固定 `api_version: "v1"`，并必须进入生成的 OpenAPI。`GET /metrics` 是 Prometheus 文本协议端点：它不返回 JSON、不是 `/api/v1`、不进入 OpenAPI；其 registry 仅暴露聚合指标，不携带 request body、凭据或个人数据。

OpenAPI 变更必须运行生成和契约校验。客户端只能依赖 `code` 和 `data` 结构，不能依赖 `message` 文案。

## 测试门禁

集成测试至少覆盖：

- 200 和 201 成功响应
- 空数据和分页响应
- 400、401、403、404、409、429
- 外部服务 502、503、504
- 未识别错误 500
- panic 恢复
- `data` 始终存在
- 内部错误信息不泄露
- `X-Request-ID` 响应头存在

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-14 | 确立三字段 Result、全局错误处理和业务码分段 |
| v1.1 | 2026-07-16 | 明确业务 OpenAPI 与非业务运维探针的边界 |
| v1.2 | 2026-07-16 | 固定 trace/timeout/认证扩展位和 Wrap 全局错误出口，定义 capabilities 与 metrics 契约边界 |
