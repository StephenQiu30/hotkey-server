# Proposal: STE-297 — OpenAPI 跨仓验证

## Context

STE-296 完成了 Huma HTTP 路由迁移，`docs/openapi.json` 已由 `make openapi` 生成。
本 change 跟踪 **契约导出完整性验证** 和 **跨仓客户端生成兼容性**。

## Goal

1. 自动验证 `docs/openapi.json` 覆盖所有 Huma 注册的 `/api/v1/*` 路由
2. 验证 OpenAPI spec 结构完整（版本、安全方案、Schema 数量），支持客户端生成
3. 文档化 hotkey-web `@umijs/openapi` 跨仓使用模式

## Scope

- `internal/platform/http/openapi_coverage_test.go` — Go 测试验证路由覆盖
- `scripts/validate-openapi.sh` — Bash 脚本验证 spec 结构
- `Makefile` — 新增 `openapi-validate` target
- `scripts/validate-repository.sh` — 加入 openapi 检查
- `docs/cross-repo-client-generation.md` — 跨仓文档

## Out of Scope

- hotkey-web 仓库内的实际客户端生成（外部仓库）
- OpenAPI spec 内容变更（仅验证现有 spec）
