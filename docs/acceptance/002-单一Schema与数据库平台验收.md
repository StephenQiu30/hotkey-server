---
layer: Acceptance
doc_no: "002"
audience: [Dev, QA, Ops]
feature_area: 数据库运行时
purpose: 记录单一Schema之上的连接、事务、兼容性与Repository平台验收证据
canonical_path: docs/acceptance/002-单一Schema与数据库平台验收.md
status: accepted
version: v1.0
owner: HotKey Server Team
inputs:
  - docs/prd/002-单一Schema与数据库平台.md
  - docs/plans/002-单一Schema与数据库平台计划.md
  - docs/design/003-数据库与数据生命周期设计.md
commit: fba5be0833bf6fdfc6c04579589aaba8653937f0
result: accepted
---

# 单一Schema与数据库平台验收

## 结论

验收通过。提交 `fba5be0833bf6fdfc6c04579589aaba8653937f0` 在既有唯一 `db/schema.sql` 基线上建立唯一 pgx 连接池、由其派生的 GORM facade、显式事务、Repository 实现和安全数据库命令；未新增平行 Schema、Migration 或 AutoMigrate。

## 环境与 fixture

- macOS arm64、Go 1.26.3、PostgreSQL 18.4、`pg_trgm` 与 `vector`。
- `HOTKEY_TEST_DSN=postgres:///hotkey_plan002_test?sslmode=disable` 指向可丢弃测试库；每个 Go 集成场景创建、初始化并删除独立 `hotkey_it_*` 数据库，容量场景使用独立 `hotkey_capacity_*` 数据库。
- `db init --empty-only --confirm-empty` 初始化嵌入的相邻 `db/schema.sql`；`db verify` 在只读事务中确认 PostgreSQL 16+、扩展、53 张表和 canonical catalog contract。

## 红绿证据

| 验收项 | 红灯信号 | 绿灯证据 |
|---|---|---|
| Schema 兼容性 | 删除或同名替换 `monitors_relevance_threshold_check`、替换 `contents_source_published_idx`、改变列默认值，或加入非 canonical 表，`db verify` 必须失败 | 集成测试验证同名篡改 CHECK、索引、默认值及缺失约束/索引都会被拒绝；兼容库返回 53 表与 catalog fingerprint |
| 空库安全 | 已有 public table、view 或 composite type 时执行 init 不得写入 | 集成测试证明 `db init` 拒绝上述对象；初始化通过 advisory lock、事务回滚与显式确认保护 |
| 事务与连接 | 回调返回错误、panic 或活动查询被取消时不得遗留提交或失效连接 | 同一 `*sql.Tx` 的 raw SQL 与 GORM 更新提交可见；错误/panic 回滚、活动取消后的 ping、回调 context 重入拒绝均通过 |
| Repository | 乐观锁冲突、软删除、非法 cursor、SQLSTATE 与取消不得泄漏数据库细节 | 真实 PostgreSQL 覆盖 CRUD、并发版本冲突、错误映射、软删除及排序方向/筛选指纹绑定 cursor |
| 容量查询 | 不得以无界 OFFSET 完成列表页 | 1,000 行可缩放 fixture 的 `EXPLAIN` 使用 `contents_source_published_idx` 的 Index Only Scan，且脚本拒绝 OFFSET / Limit All |

## 长期质量门禁

`HOTKEY_TEST_DSN='postgres:///hotkey_plan002_test?sslmode=disable' make ci` 通过，包含：

- `go vet ./...`、全量 `go test ./...`、构建、架构/Repository 门禁与 Schema 双跑；
- `database-runtime-verify` 的空库 init/verify、独立数据库集成测试和容量计划断言；
- 复审后的 `git diff --check` 与 `make clean`，无 `hotkey` 二进制或临时数据库残留。

## 边界与残余风险

- 该任务只提供数据库平台和 Repository 具体实现；公共 HTTP/OpenAPI 属于 PLAN-003，River worker 接入属于 PLAN-013。
- 容量 fixture 可按 `HOTKEY_CAPACITY_ROWS` 扩展；9 百万内容规模的运行治理证据由 PLAN-017 负责保存。
- 本次实测环境为 PostgreSQL 18.4，满足 16+ 下限；在 PostgreSQL 16 专用 CI 环境增加同一门禁可进一步覆盖 catalog 输出差异。

## 独立审核

独立 Reviewer 复审通过：同名但语义改变的 CHECK 约束会令 `db verify` 报 contract mismatch；public composite type 会令 empty-only init 拒绝。复审同时确认全量 CI、重复集成测试、diff 检查、临时数据库清理和构建产物清理均通过。
