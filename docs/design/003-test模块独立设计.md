---
layer: Design
doc_no: 003
audience: Dev, QA
feature_area: 测试工程化
purpose: 定义 hotkey-server Go 测试目录独立方案，降低测试与生产代码耦合，保持 internal/ 目录干净
canonical_path: docs/design/003-test模块独立设计.md
status: draft
version: v1.0
owner: Codex
inputs:
  - docs/design/002-go后端技术选型规范.md
  - internal/**/*_test.go
  - cmd/api/main_test.go
outputs:
  - tests/ 目录权威定义
  - ADR-017 测试目录独立
  - 下游实施计划依据
triggers:
  - 启动测试模块迁移
  - 新增集成测试或 fake 前
downstream:
  - docs/plans/010-test模块独立计划.md
linear:
  - STE-300
---

# Test 模块独立设计

## 1. 背景与目标

### 1.1 现状

| 项 | 事实 |
|----|------|
| Go 测试文件 | 28 个 `*_test.go`，全部与源码同包（白盒） |
| 分布 | `internal/*` 27 个 + `cmd/api/main_test.go` 1 个 |
| Fake/Mock | 多数定义在测试文件内；`auth.fakeRepo` 跨 `service_test` / `http_test` 复用 |
| Fixture | `internal/platform/x/testdata/search_success.json` |
| 集成测试 | `cmd/api/main_test.go` 手动装配 router + 清表，与 `main.go` wiring 重复 |
| Runtime smoke | `scripts/smoke-api.sh`（ADR-015 保留，不迁入 `tests/`） |

### 1.2 目标

1. **`internal/` 与 `cmd/` 零 `*_test.go`**：生产目录只保留业务与装配代码。
2. **测试边界清晰**：单元测试、集成测试、fake/fixture 各有固定位置。
3. **最小生产改动**：优先通过外部测试包（`auth_test`）与 JSON 匿名 struct 替代未导出类型访问。
4. **与 L1 Fx/Wire 对齐**：集成测试 harness 可演进为 Wire test injector，避免重复手工装配。

### 1.3 非目标

- 不引入 testcontainers-go（后续 Epic 再评估）。
- 不拆独立 Go module。
- 不将 `scripts/smoke-api.sh` 改写为 Go test。
- 不在 `internal/` 保留 `testsupport/` 或 `testing/` 子包。

---

## 2. 方案选型（ADR-017）

### 2.1 候选方案

| 方案 | 概要 | 优点 | 缺点 |
|------|------|------|------|
| **A（选用）** | 分层 `tests/`：`unit/` + `integration/` + `testutil/` + `fixtures/` | 边界清晰；与 Wire test graph 自然对齐；`internal/` 完全干净 | 一次性迁移 28 文件；fake 需抽取 |
| B | 扁平 `tests/*.go` | 路径短 | 难维护；不符合按域组织习惯 |
| C | `tests/` + `internal/testsupport/` 放 fakes | fake 离 domain 近 | `internal/` 仍混入测试代码，违背目标 |

### 2.2 决策

**选用方案 A**：分层 `tests/` 目录，全部测试外移为外部包（`package xxx_test`）。

### 2.3 未导出符号处理

扫描现有测试后，仅少量依赖未导出类型（如 `auth.userResponse`）。外移策略：

- HTTP 响应断言改用 **匿名 struct + json tag**，或仅断言 status code。
- Fake 实现各包已导出 interface（如 `auth.Repository`），迁入 `tests/testutil/fake/<domain>/`。
- **不**为测试新增 `internal/*/export_test.go` 或 `testing.go`（除非迁移中发现不可替代的白盒需求）。

---

## 3. 目录结构权威定义

```text
tests/
  unit/                    # 单元 + HTTP handler 测试（外部包）
    auth/
    alert/
    config/
    content/
    jobs/
    monitor/
    notify/
    observability/
    platform/x/
    scoring/
    server/
    topic/
    trend/
  integration/
    api/                   # 原 cmd/api/main_test.go（需 DATABASE_URL）
  testutil/
    db.go                  # setupTestDB、表清理
    router.go              # setupTestRouter（后续对齐 Wire）
    env.go                 # SkipIfNoDB 等
    fake/
      auth/
      content/
      jobs/
      monitor/
      notify/
      trend/
      topic/
      ...
  fixtures/
    platform/x/
      search_success.json  # 原 internal/platform/x/testdata/
```

**包命名规则：**

| 位置 | package 名 | import 方式 |
|------|------------|-------------|
| `tests/unit/auth/` | `auth_test` | `import "…/internal/auth"` |
| `tests/integration/api/` | `api_test` | import 多个 internal 包 |
| `tests/testutil/` | `testutil` | 仅测试侧 helper，不被生产 import |
| `tests/testutil/fake/auth/` | `fakeauth` | 实现 `auth.Repository` 等 |

**生产目录（迁移后）：**

```text
internal/          # 无 *_test.go、无 testdata/
cmd/               # 无 *_test.go
scripts/           # smoke-api.sh 保留
```

---

## 4. 迁移映射

| 原路径 | 新路径 |
|--------|--------|
| `internal/auth/service_test.go` | `tests/unit/auth/service_test.go` |
| `internal/auth/http_test.go` | `tests/unit/auth/http_test.go` |
| `internal/config/config_test.go` | `tests/unit/config/config_test.go` |
| `internal/content/*_test.go` | `tests/unit/content/` |
| `internal/jobs/*_test.go` | `tests/unit/jobs/` |
| `internal/monitor/*_test.go` | `tests/unit/monitor/` |
| `internal/notify/*_test.go` | `tests/unit/notify/` |
| `internal/observability/*_test.go` | `tests/unit/observability/` |
| `internal/server/*_test.go` | `tests/unit/server/` |
| `internal/topic/*_test.go` | `tests/unit/topic/` |
| `internal/trend/*_test.go` | `tests/unit/trend/` |
| `internal/alert/service_test.go` | `tests/unit/alert/service_test.go` |
| `internal/scoring/service_test.go` | `tests/unit/scoring/service_test.go` |
| `internal/platform/x/client_test.go` | `tests/unit/platform/x/client_test.go` |
| `internal/platform/x/testdata/` | `tests/fixtures/platform/x/` |
| `cmd/api/main_test.go` | `tests/integration/api/main_test.go` |

---

## 5. 与 ADR-015 / 002 目录的关系

| 层级 | 职责 | 位置 |
|------|------|------|
| 单元 / handler | 快速反馈、无外部依赖（或 fake） | `tests/unit/` |
| 集成 | 真实 PG、完整 router | `tests/integration/` |
| Runtime smoke | 二进制启动、端到端探活 | `scripts/smoke-api.sh` |
| 门禁 | `make test` + `make validate` | `Makefile` / `scripts/validate-repository.sh` |

`go test ./...` 继续覆盖 `tests/` 下所有包；`Makefile` 的 `test` target 语义不变。

---

## 6. 风险与缓解

| 风险 | 缓解 |
|------|------|
| 外部包无法访问未导出符号 | 迁移前扫描；匿名 struct / 导出 interface 已足够 |
| Fake 抽取工作量大 | 按域分批迁移；先 `testutil/fake` 再移测试文件 |
| 集成测试 wiring 与 main 漂移 | `testutil/router.go` 单点维护；L1 Wire 后改为 test injector |
| Fixture 路径变更 | 统一 `testutil/fixturePath()` 或 `//go:embed` |
| CI 遗漏新目录 | `go test ./...` 已包含；`validate-repository.sh` 无需改 |

---

## 7. 验收标准

1. `internal/` 与 `cmd/` 下无任何 `*_test.go` 与 `testdata/`。
2. `tests/` 目录结构符合第 3 节定义。
3. `go test ./...` 全绿（`DATABASE_URL` 未设时集成测试 skip，行为与现网一致）。
4. `make validate` 全绿（含 smoke）。
5. `docs/design/002-go后端技术选型规范.md` ADR-015 与目录结构已同步。

---

## 8. ADR 索引更新

| ID | 标题 |
|----|------|
| ADR-017 | 测试目录独立：`tests/` 分层 + 外部测试包 |
