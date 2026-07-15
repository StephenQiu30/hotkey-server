---
layer: Plan
doc_no: "003"
audience: [Dev, QA, Ops]
feature_area: HTTP与可观测
purpose: 实施统一 Result、错误、安全中间件、指标与 OpenAPI 基础
canonical_path: docs/plans/003-HTTP契约安全与可观测基础计划.md
status: archived
execution_status: done
review_status: approved
version: v1.2
owner: HotKey Server Team
inputs:
  - docs/prd/003-HTTP契约安全与可观测基础.md
  - docs/design/004-Result响应与全局异常设计.md
  - docs/plans/001-模块化单体启动与工程门禁计划.md
  - docs/plans/002-单一Schema与数据库平台计划.md
outputs:
  - 统一 HTTP 契约
  - 安全与可观测基础
  - OpenAPI 生成门禁
triggers:
  - PRD-003 accepted 且 ready
downstream:
  - docs/acceptance/003-HTTP契约安全与可观测基础验收.md
depends_on: [PLAN-001, PLAN-002]
---

# HTTP 契约、安全与可观测基础计划

## 计划目标

让后续业务模块只注册 DTO、路由和领域错误，不再自行定义 JSON、日志、指标或异常处理。

## 开工条件

- 当前 Plan 的 status 为 accepted、review_status 为 approved、execution_status 为 ready
- 对应 PRD 的 status 为 accepted，execution_status 为 ready
- frontmatter 中 depends_on 列出的 Plan 全部为 done
- main 已同步，工作区只包含当前任务相关文件

## 执行文件

| 动作 | 路径 | 目的 |
|---|---|---|
| 修改 | cmd/hotkey/main.go | 唯一 Swaggo 总注解与 API 标题、版本、base path |
| 修改 | internal/platform/http/result.go | 泛型 Result 与分页数据 |
| 创建 | internal/platform/http/handler.go | 统一 Handler 包装与 AppError 写出 |
| 创建 | internal/platform/http/error_handler.go | 稳定错误码、错误分类与 HTTP 映射 |
| 创建 | internal/platform/http/middleware.go | 固定顺序的 request ID、trace、访问日志、recovery、CORS 与 request context/timeout |
| 创建 | internal/platform/http/capabilities.go | `GET /api/v1/capabilities` DTO、Handler 与 Swaggo 注解 |
| 修改 | internal/platform/http/router.go、server.go | 全局路由、`/metrics` 挂接、超时与优雅停机 |
| 创建 | internal/platform/observability/metrics.go | 专属 Prometheus registry、HTTP/依赖指标与 `/metrics` handler |
| 创建 | internal/platform/observability/tracing.go | OTel Resource、TracerProvider、propagator、可注入 exporter 与 shutdown |
| 修改 | internal/platform/config/config.go、internal/bootstrap/app.go | 观测配置、Fx provider、生命周期与 API 装配 |
| 修改 | internal/shared/errors/error.go | 稳定业务错误码、重试语义与重复码防护 |
| 创建/修改 | internal/platform/http/*_test.go | Result、错误、状态矩阵、中间件、脱敏、metrics、trace 与 capabilities 测试 |
| 创建 | internal/platform/observability/*_test.go | provider/resource/shutdown 与自定义 registry 测试 |
| 创建 | docs/openapi/swagger.json | 由 Swaggo 生成并受版本控制的 Swagger/OpenAPI 2.0 公共契约 |
| 创建 | tools/tools.go | `tools` build tag 下锁定 Swaggo generator 模块版本 |
| 创建 | tests/architecture/openapi_test.go | capabilities 正向路径和 `/healthz`、`/readyz`、`/metrics` 排除断言 |
| 修改 | scripts/validate-architecture.sh | 仅允许 `internal/platform/http/result.go` 直接调用 Gin JSON 输出；业务 transport 禁止绕过统一 Handler |
| 修改 | Makefile | `openapi`、`openapi-validate`、`openapi-check` 与 CI 集成 |
| 修改 | go.mod、go.sum | Prometheus、OpenTelemetry SDK/OTLP 与锁定版本的 Swaggo 生成器依赖 |

## 执行步骤

1. 先为 Result 三字段、空/分页数据、11 个 HTTP 状态、panic、`X-Request-ID`、超时、脱敏、每个指标与 trace 属性写红灯；所有失败 Result 必须断言 `data: null`。
2. 在 `internal/shared/errors` 固定并测试唯一的基础码：`10000 invalid_request`、`10001 validation_failed`、`10002 conflict`、`10003 not_found`、`10004 rate_limited`、`20000 unauthenticated`、`20001 forbidden`、`90000 internal`、`90001 unavailable`、`90002 bad_gateway`、`90003 deadline_exceeded`。本任务的 `401`/`403` 只映射中性错误，不读取 token 或执行权限判断。
3. 以 `HandlerFunc func(*gin.Context) error` 和 `Wrap` 统一写 Result；`context.Canceled`、deadline、AppError 与未知错误必须分别映射，业务/未来 transport 文件不得直接调用 `c.JSON`、`c.AbortWithStatusJSON` 或 `c.String`。
4. 固定 middleware 顺序为 `RequestID -> TraceContext -> AccessLog -> Recovery -> CORS -> RequestContextTimeout -> Authentication(passthrough) -> Authorization(passthrough) -> Handler -> GlobalErrorHandler(Wrap)`。PLAN-003 的两个认证扩展位不得解析 token 或执行权限规则；Request ID 复用合法入站值或生成 UUID，写响应头、Zap 字段和 span 属性；日志 redactor 必须遮蔽 `Authorization`、`Cookie`、`Set-Cookie`、`X-API-Key` 和请求正文。
5. 创建应用专属 `prometheus.Registry`，注册 `hotkey_http_requests_total{method,route,status}`、`hotkey_http_request_duration_seconds{method,route,status}`、`hotkey_http_panics_total{route}` 与 `hotkey_dependency_health{dependency}`；仅在 `/metrics` 使用 `promhttp.HandlerFor` 暴露它。metrics 不是 JSON、不是 `/api/v1`，也不进入 OpenAPI。
6. 创建 `Telemetry` provider：Resource 的 `service.name=hotkey-server`，全局 W3C TraceContext propagator，默认无 exporter 时使用 no-op SDK，配置 OTLP HTTP endpoint 时创建 batch span processor。通过 Fx `OnStop` 调用 provider shutdown，HTTP middleware 从入站 headers 提取/传播 context，并将 `http.request_id`、method、route、status 属性写入 span。
7. 创建 `GET /api/v1/capabilities` 作为唯一稳定的非业务契约对象，返回 `Result[Capabilities]{data:{api_version:"v1"}}`。将 `@Summary`、`@Tags`、`@Produce json`、200 Result schema 和 `/api/v1` router group 写在实际 handler；`/healthz`、`/readyz`、`/metrics` 不加 Swaggo 操作注解。
8. 在 `cmd/hotkey/main.go` 增加 Swaggo 总注解；`make openapi` 使用锁定的 `github.com/swaggo/swag/cmd/swag` 版本和 `--parseInternal --outputTypes json` 生成 `docs/openapi/swagger.json`。`make openapi-validate` 运行架构测试，`make openapi-check` 依次生成、验证，并以 `git diff --exit-code -- docs/openapi/swagger.json` 拒绝漂移；`make ci` 必须依赖该门禁。
9. 完成 HTTP、observability、architecture 与 bootstrap 生命周期测试；运行带 `HOTKEY_TEST_DSN` 的 `make ci`，再执行 `make clean`、OpenAPI diff 和工作区检查。

## 实施任务（TDD）

### Task 1：Result、错误码与统一 Handler

**Files:**

- 修改：`internal/platform/http/result.go`、`internal/shared/errors/error.go`
- 创建：`internal/platform/http/handler.go`、`internal/platform/http/error_handler.go`、`internal/platform/http/handler_test.go`

**Interfaces:**

```go
type HandlerFunc func(*gin.Context) error

func Wrap(handler HandlerFunc) gin.HandlerFunc
func WriteError(c *gin.Context, err error)

type Page[T any] struct {
	Items []T `json:"items"`
	Total int64 `json:"total"`
	Page int `json:"page"`
	PageSize int `json:"page_size"`
}
```

- [ ] 写 `TestResultAlwaysHasCodeMessageAndData`、`TestWriteErrorStatusMatrix` 和 `TestWrapRecoversPanic`：用 table case 断言 400/401/403/404/409/429/500/502/503/504 的 HTTP status、稳定业务码、响应顶层键严格为 `code,message,data`，失败 data 为 JSON `null`。
- [ ] 运行 `go test ./internal/platform/http -run 'Test(Result|WriteError|Wrap)' -count=1`，预期因缺少 `Wrap`、分页或稳定映射失败。
- [ ] 在 `internal/shared/errors/error.go` 声明 11 个唯一常量和显式 `RegisterCode` catalog；初始化时注册基础码、拒绝重复码，未来领域只能通过该 registry 扩展。`New`/`Wrap` 仅构造已注册的码；在 `WriteError` 中保留 `*AppError`，将 deadline 映射为 `90003/504`，未知错误映射为 `90000/500`。实现最小 Handler：

```go
func Wrap(handler HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := handler(c); err != nil {
			WriteError(c, err)
		}
	}
}
```

- [ ] 重跑同一命令，预期 PASS；提交 `feat: add HTTP result and error contract`。

### Task 2：中间件、Prometheus 与 Trace 生命周期

**Files:**

- 创建：`internal/platform/http/middleware.go`、`internal/platform/http/middleware_test.go`
- 创建：`internal/platform/observability/metrics.go`、`internal/platform/observability/metrics_test.go`、`internal/platform/observability/tracing.go`、`internal/platform/observability/tracing_test.go`
- 修改：`internal/platform/config/config.go`、`internal/bootstrap/app.go`、`internal/platform/http/router.go`、`internal/platform/http/server.go`

**Interfaces:**

```go
type Metrics struct {
	Registry *prometheus.Registry
}

func NewMetrics() (*Metrics, error)
func (m *Metrics) Handler() http.Handler

type Telemetry struct {
	TracerProvider *sdktrace.TracerProvider
}

func NewTelemetry(config.Config) (*Telemetry, error)
func (t *Telemetry) Shutdown(context.Context) error
```

- [ ] 写 middleware 红灯：给定合法 `X-Request-ID` 保持原值；缺失时生成 UUID；panic 返回 `90000/500`；deadline handler 返回 `90003/504`；Zap observer 中的 Authorization、Cookie、Set-Cookie、X-API-Key 和 body 不出现明文。
- [ ] 写 metrics/trace 红灯：请求后 `Registry.Gather()` 含 4 个 `hotkey_*` 指标及 method/route/status 标签；入站 traceparent 被提取，span 带 `http.request_id`；假 exporter 在 Fx stop 被 shutdown。
- [ ] 运行 `go test ./internal/platform/http ./internal/platform/observability -run 'Test(Middleware|AccessLog|Metrics|Trace)' -count=1`，预期缺少 middleware/provider/registry 失败。
- [ ] 实现固定链：`RequestID -> TraceContext -> AccessLog -> Recovery -> CORS -> RequestContextTimeout -> Authentication(passthrough) -> Authorization(passthrough) -> Handler -> GlobalErrorHandler(Wrap)`。两个 passthrough 不读取 Authorization header；只有 `/metrics` 使用 `promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{})`；Telemetry 在没有 OTLP endpoint 时不配置 exporter，在 endpoint 存在时创建 OTLP HTTP batch exporter；通过 Fx `OnStop` 调用 `Shutdown`。
- [ ] 重跑红灯命令，预期 PASS；提交 `feat: add HTTP observability foundation`。

### Task 3：真实 OpenAPI 契约对象与确定性生成

**Files:**

- 修改：`cmd/hotkey/main.go`、`internal/platform/http/router.go`
- 创建：`internal/platform/http/capabilities.go`、`internal/platform/http/capabilities_test.go`
- 创建：`docs/openapi/swagger.json`、`tests/architecture/openapi_test.go`、`tools/tools.go`
- 修改：`Makefile`、`go.mod`、`go.sum`

**Interfaces:**

```go
type Capabilities struct {
	APIVersion string `json:"api_version"`
}

func CapabilitiesHandler(c *gin.Context) error
```

- [ ] 写 `TestCapabilitiesRoute`：`GET /api/v1/capabilities` 返回 200、`code=0`、`data.api_version="v1"`；`/healthz`、`/readyz` 保持 Result，`/metrics` 保持 Prometheus content type。
- [ ] 写 `TestOpenAPIContract`：解析受版本控制的 `docs/openapi/swagger.json`，断言只有公开路径 `/api/v1/capabilities`，其 200 response 引用具体 Result data schema，并断言 `/healthz`、`/readyz`、`/metrics` 不存在。
- [ ] 运行 `go test ./internal/platform/http ./tests/architecture -run 'Test(Capabilities|OpenAPI)' -count=1`，预期 capabilities handler 和 OpenAPI 文件缺失失败。
- [ ] 为实际 handler 写 Swaggo 注解，并在 `cmd/hotkey/main.go` 写 `@title HotKey API`、`@version 1.0`、`@BasePath /`。新增 Make targets：

```make
openapi:
	go run github.com/swaggo/swag/cmd/swag init --generalInfo cmd/hotkey/main.go --parseInternal --output docs/openapi --outputTypes json
openapi-validate:
	go test ./tests/architecture -run TestOpenAPIContract -count=1
openapi-check: openapi openapi-validate
	git diff --exit-code -- docs/openapi/swagger.json
```

- [ ] 在 `tools/tools.go` 以 `//go:build tools` 和空白导入 `github.com/swaggo/swag/cmd/swag`，将 generator 固定为 `github.com/swaggo/swag v1.16.6` 的 go.mod 直接依赖；运行 `make openapi-check`，预期 PASS；提交 `docs: generate HTTP API contract`。

### Task 4：绕过防护与最终门禁

**Files:**

- 修改：`scripts/validate-architecture.sh`、`Makefile`
- 测试：`tests/architecture/openapi_test.go`、`internal/bootstrap/app_test.go`

- [ ] 写架构红灯：在临时业务 transport 文件中直接出现 `c.JSON(`、`c.AbortWithStatusJSON(` 或 `c.String(` 时，`scripts/validate-architecture.sh` 必须失败；平台 `result.go` 是唯一允许写 JSON 的文件。
- [ ] 修改 validator：只扫描 `internal/modules/**/transport/http/*.go`，排除 `*_test.go`，报告违反路径和行号；在 Make 的 `ci` 依赖链加入 `openapi-check`，保持 `test` 的 `HOTKEY_TEST_DSN` 显式前置条件。
- [ ] 运行 `sh scripts/validate-architecture.sh && make openapi-check && HOTKEY_TEST_DSN="$HOTKEY_TEST_DSN" make ci && make clean`，预期全通过且无 `hotkey` 二进制。
- [ ] 检查 `git diff --check`、`git status --short`，仅 staging 当前任务文件；提交 `build: gate HTTP contract and architecture boundary`。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | `go test ./internal/platform/http ./internal/platform/observability -count=1` | 新契约测试因 Handler、middleware、registry、provider 或 capabilities 路由缺失失败 |
| 状态矩阵 | `go test ./internal/platform/http -run 'Test(Result|Handler|Middleware|Capabilities)' -count=1` | 200、201、400、401、403、404、409、429、500、502、503、504、panic、timeout 与 `data` 存在性通过 |
| 观测 | `go test ./internal/platform/observability ./internal/platform/http -run 'Test(Metrics|Trace|AccessLog)' -count=1` | 自定义 registry、指标标签、trace Request ID、provider shutdown 与脱敏通过 |
| 契约 | `make openapi-check` | 生成稳定；`GET /api/v1/capabilities` 存在，`/healthz`、`/readyz`、`/metrics` 不在契约中 |
| 架构 | `sh scripts/validate-architecture.sh` | 非平台业务 transport 无直接 Gin JSON 输出 |
| 全量 | `HOTKEY_TEST_DSN="$HOTKEY_TEST_DSN" make ci` | OpenAPI 门禁、数据库、lint、全量测试、构建和全部架构门禁通过 |

## 验收清单

- 所有 JSON 顶层只有 code、message、data
- data 在成功与失败响应中始终存在
- X-Request-ID 只在响应头、日志和链路中出现
- `/api/v1/capabilities` 是唯一生成的 API 路径并返回 `api_version: "v1"`；`/healthz`、`/readyz`、`/metrics` 均不在 OpenAPI
- panic 与内部错误不泄露堆栈、SQL、密钥或 Provider 原文
- 400、401、403、404、409、429、500、502、503、504 契约稳定
- Prometheus 自定义 registry 覆盖请求量、延迟、状态、panic 与依赖健康；Telemetry provider 在 Fx stop 时关闭
- 架构门禁阻止 Controller 直接输出 JSON
- OpenAPI 只发布 `/api/v1` 的 capabilities 契约；healthz 和 readyz 继续遵守 Result 与脱敏契约但不被发布，metrics 保持文本协议且不被发布

## 提交边界

- test: 定义 HTTP 与观测契约
- impl: 实现统一 HTTP、安全和观测基础
- docs: 生成并校验 OpenAPI


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现

## 实施变更复核

- 2026-07-16：独立 Reviewer 要求修改。原计划没有实际 `/api/v1` 契约对象、OpenAPI 生成/漂移门禁、Prometheus/OTel Fx 装配、认证边界和当前 `HOTKEY_TEST_DSN` CI 命令。本次修订明确唯一 capabilities 契约、`/metrics`、provider 生命周期、错误码和测试矩阵；审核状态为 `changes_requested`，待独立复核。
- 2026-07-16：复核发现中间件顺序、认证延后和错误出口不能仅在 PRD/Plan 改写。先将同一最终契约写入 Design-004 v1.2，再同步本计划的 passthrough 扩展位与 `Wrap` GlobalErrorHandler，待最终独立审核。
- 2026-07-16：独立 Reviewer 最终批准。Design-004 v1.2、PRD-003 与本计划的中间件、认证边界、OpenAPI 与 metrics 契约一致；Plan 进入 `ready`。
- 2026-07-16：实现完成后的独立 Reviewer 最终验收批准。AST 门禁覆盖任意 `*gin.Context` 参数、JSON 输出方法与赋值别名；访问和 panic 日志均有 `module`，当前平台默认 `platform`，未来模块可调用 `SetModule`。无 P0/P1。
