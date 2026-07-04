---
layer: Plan
doc_no: 020
audience: Dev, QA, Ops
feature_area: 契约与HTTP规范收敛
purpose: 定义 DTO 归位、鉴权表达、错误响应契约化和 OpenAPI 产物生成的落地步骤与门禁
canonical_path: docs/plans/020-契约与HTTP规范收敛计划.md
status: review
version: v1.0
owner: Codex
inputs:
  - docs/design/011-接口契约与任务运行设计.md
  - docs/plans/018-后端工程化落地计划.md
outputs:
  - OpenAPI 主线落地
  - Gin HTTP 规范统一
triggers:
  - 需要冻结服务端接口契约主线
downstream:
  - docs/acceptance/004-后端工程化架构验收.md
---

# 背景

如果 DTO、鉴权表达和错误响应不先按统一规则落地，`docs/swagger.json` 很容易沦为“有产物但不可信”的形式文档。

# 目标

1. 建立模块内 DTO 归位规则。
2. 建立统一鉴权和错误响应在 OpenAPI 中的表达。
3. 生成并校验 `docs/swagger.json`。

# 非目标

1. 本计划不覆盖数据库迁移。
2. 本计划不覆盖 Worker 调度实现。

# Task 1: DTO 与 route metadata 归位

目标：

1. 为 Gin handler 定义统一 DTO 位置。
2. 为 route metadata 建立统一归位方式。

验证门禁：

```bash
go test ./internal/platform/http/... ./...
```

# Task 2: 鉴权与错误响应契约化

目标：

1. 在契约中统一 Bearer/JWT 表达。
2. 在契约中统一错误响应结构。
3. 将关键枚举约束映射到 schema。

验证门禁：

```bash
go test ./internal/platform/http/... ./...
```

# Task 3: OpenAPI 产物生成与校验

目标：

1. 生成 `docs/swagger.json`。
2. 验证产物可被下游客户端生成工具消费。

验证门禁：

```bash
test -f docs/swagger.json
```

# 风险与边界

1. 如果 DTO 直接复用 ORM 模型，契约会被持久化细节污染。
2. 如果契约表达不统一，Web 与 Miniapp 生成客户端会频繁返工。

# 变更记录

## v1.0

1. 新建契约与 HTTP 规范收敛计划。
2. 将 DTO、鉴权、错误响应和 OpenAPI 产物生成拆成独立执行阶段。
