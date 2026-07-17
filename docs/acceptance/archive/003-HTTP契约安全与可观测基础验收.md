---
layer: Acceptance
doc_no: "003"
audience: [Dev, QA, Ops]
feature_area: HTTP与可观测
purpose: 记录统一 Result、HTTP 安全边界、观测基础与 OpenAPI 门禁的长期验收证据
canonical_path: docs/acceptance/archive/003-HTTP契约安全与可观测基础验收.md
status: accepted
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/archive/003-HTTP契约安全与可观测基础.md
  - docs/plans/archive/003-HTTP契约安全与可观测基础计划.md
  - docs/design/archive/004-Result响应与全局异常设计.md
commit: 1f3709e81b60b50735249ee452964a86e975f8d9
result: accepted
---

# HTTP 契约、安全与可观测基础验收

## 结论

验收通过。实现提交范围 `241f3ed..1f3709e` 建立唯一的三字段 JSON Result、稳定错误码和统一 Handler；固定 Request ID、W3C trace、访问日志、panic recovery、CORS、请求超时与认证/授权 passthrough 链；并交付专属 Prometheus registry、OpenTelemetry Fx 生命周期、受版本控制的 OpenAPI 以及模块 transport JSON 输出门禁。未实现 token 解析、会话或授权规则，认证职责仍由 PLAN-004 交付。

## 环境与 fixture

- macOS arm64、Go 1.26.3、PostgreSQL 18.4（`pg_trgm`、`vector`）。
- `HOTKEY_TEST_DSN=postgres:///hotkey_plan002_test?sslmode=disable` 指向可丢弃测试库；全量 CI 复用既有数据库运行时 fixture，创建并清理独立测试库。
- 运行时新增 Prometheus `v1.23.2`、OpenTelemetry `v1.43.0` 与锁定的 Swaggo generator `v1.16.6`；未配置 `HOTKEY_OTLP_HTTP_ENDPOINT` 时不创建 exporter，测试以内存/假 exporter 复核 trace 与 Fx shutdown。

## 红绿证据

| 验收项 | 红灯信号 | 绿灯证据 |
|---|---|---|
| Result 与错误边界 | `PageOK`、11 个注册基础码和 `Wrap`/`WriteError` 缺失时 HTTP 契约测试不能编译 | Result 顶层严格为 `code`、`message`、`data`；覆盖 400、401、403、404、409、429、500、502、503、504、deadline 与 panic，失败 `data` 为 `null` 且不泄露 Cause |
| 中间件与日志安全 | request ID、超时、recovery、metrics/tracing provider 尚未装配时 middleware/observability 测试失败 | 合法 `X-Request-ID` 被保留、缺失时生成 UUID；W3C 入站 parent 被提取，span 带 request ID；访问和 panic 日志均有 request ID、module、route、状态、耗时且不含 Authorization、Cookie、Set-Cookie、X-API-Key 或 body 明文 |
| Prometheus 与 Fx 生命周期 | 无专属 registry、指标向量或 provider shutdown 时 metrics/trace 测试失败 | `/metrics` 仅输出 Prometheus 文本；registry 包含 `hotkey_http_requests_total`、`hotkey_http_request_duration_seconds`、`hotkey_http_panics_total`、`hotkey_dependency_health`；Fx stop 会 flush/shutdown telemetry exporter |
| OpenAPI 契约 | capabilities 路由缺失时返回 404，且 `docs/openapi/swagger.json` 不存在 | `GET /api/v1/capabilities` 返回 `code=0` 和 `data.api_version="v1"`；生成的 Swagger 2.0 仅含该路径及具体 `Result[Capabilities]` schema，明确排除 `/healthz`、`/readyz`、`/metrics` |
| JSON 输出架构边界 | 旧文本匹配会接受 `ctx.JSON(...)` | AST 门禁识别任意 `*gin.Context` 参数及其直接赋值别名，拒绝 `JSON`、`AbortWithStatusJSON`、`String`；模块 transport 只能使用平台 Result 工具和 `Wrap` |

## 长期质量门禁

`HOTKEY_TEST_DSN='postgres:///hotkey_plan002_test?sslmode=disable' make ci` 通过，覆盖：

- `make openapi-check` 的确定性生成、schema 契约和 Git 漂移检查；
- `go vet ./...`、全量 `go test ./...`、HTTP/observability/Fx 生命周期、OpenAPI 与架构门禁；
- 真实 PostgreSQL 的既有运行时、Schema 与 Repository 回归验证；
- 构建、`make clean`、`git diff --check` 与工作区清洁检查。

## 边界与残余风险

- `/metrics` 是聚合文本协议，不进入 OpenAPI，也不携带 body、凭据或业务数据；`/healthz` 与 `/readyz` 保持安全 Result 但不公开为 API 操作。
- 本任务仅提供 Authentication/Authorization passthrough 与 401/403 中性错误映射；PLAN-004 必须在该扩展点注入 token、会话和权限规则。
- 未对外部 OTLP collector 做联网 smoke test；可选 exporter 的资源、传播和 Fx shutdown 已通过内存/假 exporter 测试，实际 endpoint 连通性由部署运行验证负责。

## 独立审核

独立 Reviewer 在实现完成后发现并关闭两项 P1：JSON 输出门禁已从接收者名称匹配升级为 Gin Context AST 检查，访问和 panic 日志已具备 `module` 字段、`platform` 默认值与未来模块 `SetModule` 扩展点。复审结论为无 P0/P1，批准归档。
