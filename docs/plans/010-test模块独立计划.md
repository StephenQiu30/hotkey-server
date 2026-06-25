---
layer: Plan
doc_no: 010
audience: Dev, QA
feature_area: 测试工程化
purpose: 按 003 设计将 28 个 Go 测试外移至 tests/，保持 make validate 全绿
canonical_path: docs/plans/010-test模块独立计划.md
status: partial
version: v1.1
owner: Codex
inputs:
  - docs/design/003-test模块独立设计.md
  - docs/design/002-go后端技术选型规范.md
outputs:
  - tests/ 目录落地
  - internal/ 测试文件清零
triggers:
  - 003 设计评审通过
downstream: []
---

> **路径注记：** 文中 `internal/server` 等为迁移前示例。`tests/` 外移已基本完成；工程主线见 [`006`](../design/006-Go后端工程与启动架构设计.md)。

# Test 模块独立实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development — 每批迁移后 `go test` 必须全绿再进入下一批。

**Goal:** 将全部 28 个 `*_test.go` 迁至 `tests/`，`internal/` 与 `cmd/` 无测试文件。

**Architecture:** 方案 A — `tests/unit` + `tests/integration` + `tests/testutil` + `tests/fixtures`。

**Design:** [`docs/design/003-test模块独立设计.md`](../design/003-test模块独立设计.md)

**Linear:** [STE-300](https://linear.app/stephenqiu/issue/STE-300/测试-test-模块独立tests-目录外移)

---

### Phase 0: 脚手架与文档

**Files:**
- Create: `tests/testutil/env.go`, `db.go`, `router.go`
- Create: `tests/testutil/fake/` 目录骨架
- Modify: `docs/design/002-go后端技术选型规范.md`（ADR-015 + 目录结构）

- [ ] **Step 0.1:** 创建 `tests/` 目录骨架（unit/integration/testutil/fixtures）
- [ ] **Step 0.2:** 从 `cmd/api/main_test.go` 抽取 `setupTestDB` / `setupTestRouter` 到 `testutil`
- [ ] **Step 0.3:** 更新 002 ADR-015 与 §5 目录结构

**Validation:**

```bash
go test ./tests/testutil/...  # 空包或 helper 编译通过
```

---

### Phase 1: 抽取共享 Fakes

**Scope:** 将各包测试内 fake 迁入 `tests/testutil/fake/<domain>/`

**Priority domains:** `auth` → `jobs` → `notify` → `content` → `monitor` → `trend` → `topic`

- [ ] **Step 1.1:** `fakeauth.Repo`（原 `auth.fakeRepo`）
- [ ] **Step 1.2:** `fakejobs.*`（dispatch、poll_monitor、aggregate_topics 等）
- [ ] **Step 1.3:** 其余域 fake 按映射表抽取

**Validation:**

```bash
go build ./tests/testutil/...
```

---

### Phase 2: 迁移单元测试（按域分批）

每批：移动文件 → 改 `package xxx_test` → 修 import → 删原文件 → `go test`

| 批次 | 源 | 目标 |
|------|-----|------|
| 2a | `internal/auth/*_test.go` | `tests/unit/auth/` |
| 2b | `internal/config/*_test.go` | `tests/unit/config/` |
| 2c | `internal/content/*_test.go` | `tests/unit/content/` |
| 2d | `internal/jobs/*_test.go` | `tests/unit/jobs/` |
| 2e | `internal/monitor/*_test.go` | `tests/unit/monitor/` |
| 2f | `internal/notify/*_test.go` | `tests/unit/notify/` |
| 2g | `internal/observability/*_test.go` | `tests/unit/observability/` |
| 2h | `internal/server/*_test.go` | `tests/unit/server/` |
| 2i | `internal/topic/*_test.go` | `tests/unit/topic/` |
| 2j | `internal/trend/*_test.go` | `tests/unit/trend/` |
| 2k | `internal/alert/*_test.go`, `internal/scoring/*_test.go` | `tests/unit/alert/`, `tests/unit/scoring/` |
| 2l | `internal/platform/x/*_test.go` + testdata | `tests/unit/platform/x/` + `tests/fixtures/` |

- [ ] **Step 2:** 完成 2a–2l 全部批次

**每批 Validation:**

```bash
go test ./tests/unit/<domain>/...
go test ./...   # 确认无残留旧测试包失败
```

---

### Phase 3: 迁移集成测试

**Files:**
- Move: `cmd/api/main_test.go` → `tests/integration/api/main_test.go`
- Delete: `cmd/api/main_test.go`

- [ ] **Step 3.1:** 集成测试改用 `testutil.SetupTestDB` / `testutil.SetupTestRouter`
- [ ] **Step 3.2:** 删除 `cmd/api/main_test.go`

**Validation:**

```bash
# DATABASE_URL 已配置时
go test ./tests/integration/api/...
# 未配置时应 skip，不 fail
go test ./tests/integration/api/... 
```

---

### Phase 4: 清理与门禁

- [ ] **Step 4.1:** 确认 `internal/`、`cmd/` 无 `*_test.go`、`testdata/`
- [ ] **Step 4.2:** `rg '_test\.go' internal/ cmd/` 无结果
- [ ] **Step 4.3:** 全量验证

**Validation:**

```bash
make test
make lint
make validate
```

---

### 与 Go 框架工程化的关系

| 时机 | 说明 |
|------|------|
| **可独立于 L1 先行** | 测试外移不依赖 Fx/Wire |
| **L1 完成后** | `testutil/router.go` 改为调用 Wire test injector，消除与 main 双轨 |
| **L3 完成后** | 集成测试可扩展 Redis/asynq 探针 |

建议：**在 STE-295（L1）开工前或并行完成本计划**，避免 L1 改动与测试搬迁冲突。

---

### 任务清单摘要

| Phase | 任务数 | 验收 |
|-------|--------|------|
| 0 脚手架 | 3 | testutil 编译 |
| 1 Fakes | 3+ | fake 包编译 |
| 2 单元迁移 | 12 批 | 每批 `go test` 绿 |
| 3 集成 | 2 | integration 绿/skip |
| 4 清理 | 3 | `make validate` 全绿 |
