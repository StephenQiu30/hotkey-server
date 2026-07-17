---
layer: PRD
prd_no: "003"
doc_no: "003"
title: HTTP契约安全与可观测基础
audience: [PM, Dev, QA, Ops]
feature_area: HTTP与可观测
purpose: 定义统一 HTTP 契约、安全中间件和可观测基础
phase: F0
priority: P0
status: archived
execution_status: done
version: v1.2
owner: HotKey Server Team
depends_on: [PRD-001, PRD-002]
design_refs:
  - docs/design/archive/002-后端单体架构设计.md
  - docs/design/archive/004-Result响应与全局异常设计.md
canonical_path: docs/prd/archive/003-HTTP契约安全与可观测基础.md
inputs:
  - docs/design/archive/002-后端单体架构设计.md
  - docs/design/archive/004-Result响应与全局异常设计.md
outputs:
  - HTTP 契约与可观测基础需求
triggers:
  - Result、错误码、中间件或观测契约变化
downstream:
  - docs/plans/archive/003-HTTP契约安全与可观测基础计划.md
  - docs/acceptance/archive/003-HTTP契约安全与可观测基础验收.md
---

# HTTP 契约、安全与可观测基础

## 目标

建立所有业务模块共享的 HTTP、错误、安全、日志、指标和链路基础，防止后续 API 各自定义响应与中间件。

## 范围

- 实现泛型 Result，顶层只包含 code、message、data，且 data 始终存在。
- 建立 AppError、领域错误到 HTTP/业务码映射和统一 Handler 包装。
- 固化 request ID、panic recovery、访问日志、认证上下文、超时和 CORS 等中间件顺序。
- 接入结构化 Zap、OpenTelemetry 和 Prometheus 基础能力。
- 建立 Swaggo 生成与 OpenAPI 契约校验入口。
- 建立唯一的非业务契约路由 `GET /api/v1/capabilities`，返回当前公开 API 版本与能力声明；它是本任务唯一进入 OpenAPI 的 `/api/v1` 路由，不代替后续资源 API。
- 定义分页数据、校验错误和外部依赖错误的稳定响应。
- 为 `/healthz` 与 `/readyz` 建立安全 Result 契约；它们是非 `/api/v1` 运维探针，不作为公共 OpenAPI 操作发布。
- 在非 `/api/v1` 的 `GET /metrics` 暴露 Prometheus 文本协议；它不使用 JSON Result，也不进入 OpenAPI。

## 非范围

- 不实现具体业务资源 API。
- 不解析、校验或授权访问令牌；只提供中性 request context、`401`/`403` 错误映射与中间件挂接点，具体认证授权由 PLAN-004 交付。
- 不把 request_id、堆栈、SQL、密钥或第三方原始错误放入业务 JSON。
- 不要求客户端依赖 message 文案。

## 功能要求

1. 成功业务码固定为 0，HTTP 状态保留协议语义。
2. Controller 不得直接调用 Gin JSON 输出。
3. 400、401、403、404、409、429、500、502、503、504 有稳定错误码。
4. X-Request-ID 同时进入响应头、日志和 trace 属性。
5. 指标至少覆盖请求量、延迟、状态、panic 和依赖健康。
6. OpenAPI 中所有 `/api/v1` JSON 响应声明具体 Result 数据类型。
7. 日志默认脱敏 Authorization、Cookie、来源凭据和正文。
8. `/healthz` 与 `/readyz` 使用统一 Result、错误脱敏和 Request ID，但生成的 OpenAPI 必须不包含两条路径。
9. `GET /api/v1/capabilities` 必须返回稳定 Result 和固定 `api_version: "v1"`，并作为受版本控制的生成 OpenAPI 正向契约对象。
10. `GET /metrics` 必须从应用专属 Prometheus registry 输出请求量、延迟、状态、panic 与依赖就绪度；它不属于 JSON 或公开 API 契约。
11. 观测 provider 必须以 Fx 生命周期装配：Resource 含服务名，HTTP middleware 传播 trace context 并记录 Request ID 属性，停机时在 shutdown timeout 内 flush/shutdown。

## 交付物

- HTTP Result、错误处理、中间件和分页实现。
- 日志、指标、链路基础装配及测试。
- OpenAPI 生成、校验命令和最小契约文件，以及运维探针排除断言。
- `capabilities` 契约路由、专属 Prometheus registry、OpenTelemetry provider 与可注入 exporter 生命周期。
- 错误码注册表及防重复测试。

## 验收标准

- 集成测试覆盖 200、201、空数据、分页及所有标准错误状态。
- panic 被恢复为安全 500，日志保留关联 ID 但不泄露内部详情。
- data 在成功和失败响应中始终存在。
- OpenAPI 生成前后无非确定性漂移，契约校验通过。
- 契约校验同时证明 `/api/v1` 路由被生成，而 `/healthz`、`/readyz` 未被生成。
- 契约校验证明 `GET /api/v1/capabilities` 存在，`/healthz`、`/readyz` 与 `/metrics` 均被排除；`make openapi-check` 生成后不得产生未提交漂移。
- 未经统一 Handler 的直接 JSON 输出被架构测试阻止。
- 认证尚未接入时，`401`/`403` 只由中性 AppError 映射覆盖，不存在 token 解析、身份持久化或授权规则。

## 完成定义

后续模块只注册业务路由、DTO 和错误映射，不再创建第二套响应、日志或指标机制。

## 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v1.0 | 2026-07-16 | 建立 HTTP、错误与可观测基础范围 |
| v1.1 | 2026-07-16 | 明确业务 OpenAPI 与运维探针边界 |
| v1.2 | 2026-07-16 | 增加唯一 capabilities 契约、`/metrics`、OTel 生命周期和认证延后边界，使 OpenAPI 与观测验收可执行 |
