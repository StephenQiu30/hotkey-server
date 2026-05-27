---
layer: Plan
doc_no: "23"
audience:
  - Tech-Lead
  - Dev
  - QA
feature_area: "area:n8n"
purpose: "实现 n8n 调用 hotkey-server internal API 所需的鉴权、租户上下文和幂等校验。"
canonical_path: "docs/plans/23-n8nInternalAPI鉴权与幂等实现计划.md"
status: approved
version: "1.0.0"
owner: "StephenQiu30"
inputs:
  - docs/product/prd/23-n8n外部自动化编排接入PRD.md
outputs:
  - n8n internal API 鉴权实现
  - n8n 幂等校验测试证据
triggers:
  - "internal API 鉴权或幂等策略变更"
downstream:
  - docs/plans/24-n8nWorkflow执行状态回写实现计划.md
---

# 23-n8nInternalAPI鉴权与幂等实现计划

## 1. 目标

为 `/api/v1/internal/*` 增加统一鉴权和幂等保护，保证 n8n workflow 只能以受控方式调用后端。

## 2. 文件清单

- `internal/config/config.go`
- `internal/httpapi/router.go`
- `internal/httpapi/*_test.go`
- `internal/openapi/spec.go`
- `.env.example`

## 3. 任务拆解

- 配置读取 `HOTKEY_INTERNAL_API_KEY` 和默认租户占位。
- 增加 internal API middleware，校验 `X-HotKey-Internal-Key`。
- 读取 `X-HotKey-Tenant-ID`，缺失时使用明确错误或配置默认值。
- 设计 `Idempotency-Key` 校验入口，第一阶段可先用进程内存储表达行为，后续接 Redis。
- 统一 internal API 错误结构。

## 4. TDD 与验证

- 未带 internal key 返回 401。
- internal key 错误返回 401。
- 缺少租户上下文返回明确错误。
- 相同幂等键重复请求不会重复执行写入。

## 5. 执行顺序

1. 先补配置和测试。
2. 再实现 middleware。
3. 最后接入 OpenAPI 契约和回归测试。

## 6. 回滚策略

- 可关闭 internal API 路由或回退 middleware。
- 幂等存储变更必须保持请求不重复写入的测试。

## 7. 验收命令

```bash
go test ./internal/config ./internal/httpapi ./internal/openapi
go test ./...
```

## 8. 变更记录

| 日期 | 作者 | 版本 | 变更说明 |
| --- | --- | --- | --- |
| 2026-05-27 | StephenQiu30 | 1.0.0 | 初版，按 n8n 编排接入 PRD 拆分 |
