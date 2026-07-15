---
layer: Plan
doc_no: "003"
audience: [Dev, QA, Ops]
feature_area: HTTP与可观测
purpose: 实施统一 Result、错误、安全中间件、指标与 OpenAPI 基础
canonical_path: docs/plans/003-HTTP契约安全与可观测基础计划.md
status: accepted
execution_status: backlog
review_status: pending
version: v1.1
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
| 修改 | internal/platform/http/result.go | 泛型 Result 与分页数据 |
| 创建 | internal/platform/http/handler.go | 统一 Handler 包装 |
| 创建 | internal/platform/http/error_handler.go | AppError 到 HTTP 映射 |
| 创建 | internal/platform/http/middleware.go | 中间件顺序与请求上下文 |
| 修改 | internal/platform/http/router.go | 全局错误与基础路由 |
| 修改 | internal/platform/http/server.go | 超时与优雅停机 |
| 创建 | internal/platform/observability/metrics.go | Prometheus 指标 |
| 创建 | internal/platform/observability/tracing.go | OpenTelemetry 链路 |
| 修改 | internal/shared/errors/error.go | 稳定业务错误码 |
| 创建 | internal/platform/http/*_test.go | Result、错误和中间件测试 |
| 创建 | docs/openapi/openapi.json | 生成的公共 `/api/v1` 契约文件 |
| 创建 | tests/architecture/openapi_test.go | `/api/v1` 生成与运维探针排除断言 |
| 修改 | Makefile | openapi 与 openapi-validate |
| 修改 | go.mod、go.sum | 观测与 Swagger 依赖 |

## 执行步骤

1. 先写 Result、HTTP 状态、panic、X-Request-ID 和脱敏红灯测试。
2. 扩展 AppError 与错误码注册，禁止重复业务码。
3. 实现统一 Handler 和固定中间件顺序。
4. 接入结构化日志、指标和链路，过滤敏感字段。
5. 建立只生成 `/api/v1` 业务路由的 Swaggo 与契约校验命令；为 `/healthz`、`/readyz` 写排除断言，同时保留它们的 Result、安全错误和 Request ID 测试。
6. 用最小测试路由覆盖 200、201、分页和全部错误状态。

## 验收命令

| 阶段 | 命令 | 通过标准 |
|---|---|---|
| 红灯 | go test ./internal/platform/http -count=1 | 新契约测试因处理器或中间件缺失失败 |
| 绿灯 | go test ./internal/platform/http ./internal/platform/observability -count=1 | 全部通过 |
| 契约 | make openapi && make openapi-validate | 生成稳定；所有 `/api/v1` 路由存在，`/healthz`、`/readyz` 不在契约中 |
| 全量 | make lint && make test && make build && make validate | 全部通过 |

## 验收清单

- 所有 JSON 顶层只有 code、message、data
- data 在成功与失败响应中始终存在
- X-Request-ID 只在响应头、日志和链路中出现
- panic 与内部错误不泄露堆栈、SQL、密钥或 Provider 原文
- 400、401、403、404、409、429、500、502、503、504 契约稳定
- 架构门禁阻止 Controller 直接输出 JSON
- OpenAPI 只发布 `/api/v1` 业务操作；healthz 和 readyz 继续遵守 Result 与脱敏契约但不被发布

## 提交边界

- test: 定义 HTTP 与观测契约
- impl: 实现统一 HTTP、安全和观测基础
- docs: 生成并校验 OpenAPI


## 风险与回滚

- 实现需要改变 Design 或 PRD 契约时立即停止，先更新正式文档并重新复核
- 下游 Plan 开工前，可整体回退本任务提交并同步恢复 Schema、OpenAPI 和测试
- 下游 Plan 已开工后，使用新的前向修复 Plan，不恢复旧双轨或兼容实现
