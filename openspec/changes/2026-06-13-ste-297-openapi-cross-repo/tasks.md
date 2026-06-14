# Tasks: STE-297 — OpenAPI 跨仓验证

## Task 1: Go 测试 — OpenAPI 路由覆盖验证

- [x] 1.1 创建 `internal/platform/http/openapi_coverage_test.go`
- [x] 1.2 `TestOpenAPICoverage` — 验证 docs/openapi.json 包含所有 13 个 operationId
- [x] 1.3 `TestOpenAPIFromHumaAPI` — 从 Huma API 实例生成 spec 并验证 operationId
- [x] 1.4 `TestOpenAPIVersion` — 验证版本为 3.1.0
- [x] 1.5 `TestOpenAPISecurityScheme` — 验证 BearerAuth 安全方案
- [x] 1.6 `TestOpenAPIPathCount` — 验证路径数量 >= 11

## Task 2: Bash 验证脚本

- [x] 2.1 创建 `scripts/validate-openapi.sh`
- [x] 2.2 验证文件存在且为合法 JSON
- [x] 2.3 验证 OpenAPI 版本
- [x] 2.4 验证所有 10 个 /api/v1/* 路径
- [x] 2.5 验证所有 13 个 operationId
- [x] 2.6 验证 BearerAuth 安全方案
- [x] 2.7 验证 Schema 组件数量 >= 5

## Task 3: Makefile 集成

- [x] 3.1 新增 `openapi-validate` target
- [x] 3.2 更新 `.PHONY` 列表

## Task 4: validate-repository.sh 集成

- [x] 4.1 添加 `docs/openapi.json` 到 required_files
- [x] 4.2 添加 `scripts/validate-openapi.sh` 到 required_files
- [x] 4.3 在 Runtime API smoke 前调用 `validate-openapi.sh`

## Task 5: 跨仓文档

- [x] 5.1 创建 `docs/cross-repo-client-generation.md`
