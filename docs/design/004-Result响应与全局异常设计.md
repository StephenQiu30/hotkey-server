---
layer: Design
doc_no: 004
audience: Dev, QA
feature_area: API响应与异常
purpose: 定义所有JSON接口统一使用的Result响应和全局异常转换规则
canonical_path: docs/design/004-Result响应与全局异常设计.md
status: review
version: v1.0
owner: HotKey Server Team
inputs:
  - AGENTS.md
  - docs/design/002-后端单体架构设计.md
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
-> AccessLog
-> Recovery
-> CORS
-> Authentication
-> Authorization
-> Handler
-> GlobalErrorHandler
```

`X-Request-ID` 写入响应头和日志，不加入 Result。

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

所有 JSON 接口必须声明 Result 响应。分页、详情、列表和空数据通过不同 `data` 类型表达，不创建不同顶层响应格式。

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
