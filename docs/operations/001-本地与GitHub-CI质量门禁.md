---
layer: Operations
doc_no: "001"
audience: [Dev, QA, Ops]
feature_area: 工程质量门禁
purpose: 定义本地复现与 GitHub Actions 持续集成的唯一质量门禁
canonical_path: docs/operations/001-本地与GitHub-CI质量门禁.md
status: accepted
version: v1.0
owner: HotKey Server Team
inputs:
  - Makefile
  - README.md
outputs:
  - .github/workflows/ci.yml
triggers:
  - 修改 Go 版本、Makefile CI 目标、数据库或 Redis 测试前置条件
  - 修改 GitHub Actions 工作流
downstream: []
---

# 本地与 GitHub CI 质量门禁

## 适用范围

唯一的仓库质量门禁是 `make ci`。它依次校验 OpenAPI 生成无漂移、`go vet`、真实 PostgreSQL 运行时验证、全量 Go 测试、构建、架构/仓库校验和 Schema 重复执行。不得在 GitHub Actions 中维护另一套与本地不一致的检查命令。

OpenAPI 以 Handler 上的 Swaggo 注解为唯一语义来源。`make openapi` 同时生成 `docs/openapi/docs.go` 和 `docs/openapi/swagger.json`：运行时 `/openapi.json` 从生成的 Go 注册表读取文档，JSON 文件供 CI 契约校验及下游客户端生成。两个产物必须由同一条命令重建且语义一致，禁止手工编辑。

测试源码统一存放于 `test/`，其中 `test/_suite/` 按业务包镜像保存同包单元测试与集成测试。`make test`、`make lint` 和需要指定业务包的 `go run ./test/runner test <package>` 会在进程内临时映射这些文件至被测包，随后自动删除映射；不得把 `*_test.go` 直接提交到 `internal/`。

该工作流只提供测试服务，不代表 Docker 或生产部署编排。

## GitHub Actions

[`ci.yml`](../../.github/workflows/ci.yml) 在以下情况运行：

- 推送到 `main`
- 面向 `main` 的 Pull Request
- 手动 `workflow_dispatch`

工作流使用 Go 版本文件 `go.mod`，并提供临时的 `pgvector/pgvector:pg16` 和 `redis:7-alpine` 服务。`HOTKEY_TEST_DSN` 指向可丢弃的 `hotkey_test`，测试可在其中重建 `public` schema、创建和删除子数据库；`HOTKEY_TEST_REDIS_URL` 固定使用 Redis DB 15。测试凭据只属于 Actions 临时服务，不是应用运行密钥。

## 本地复现

本地应使用具备 `pg_trgm`、`vector` 扩展和 `CREATE DATABASE` / `DROP DATABASE` 权限的可丢弃 PostgreSQL 库，并使用独立 Redis DB：

```bash
HOTKEY_TEST_DSN='postgres://USER@localhost:5432/hotkey_test?sslmode=disable' \
HOTKEY_TEST_REDIS_URL='redis://127.0.0.1:6379/15' \
make ci
make clean
```

`make ci` 会生成 `hotkey` 二进制；提交前运行 `make clean`，并确认 `git diff --check` 与 `git status --short` 没有意外产物。

## 失败处理

- OpenAPI 漂移：先执行 `make openapi`，同时提交生成的 `docs/openapi/docs.go` 与 `docs/openapi/swagger.json`，然后重跑门禁。
- 数据库失败：确认 DSN 指向可丢弃库，角色可创建/删除数据库，且启用了 `pg_trgm` 与 `vector`。
- Redis 失败：确认 URL 指向独立 DB，不复用开发验证码或限流状态。
- CI 工作流或运行时依赖变化：先更新本手册、README 和 `CONTRIBUTING.md`，再修改工作流。
