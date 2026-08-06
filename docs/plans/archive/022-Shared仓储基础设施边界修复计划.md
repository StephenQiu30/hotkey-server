---
layer: Plan
scope: backend
doc_no: "022"
audience: [Dev, QA, Ops]
feature_area: 工程目录与依赖边界
purpose: 以架构测试先行方式迁移 shared 中的 GORM/pgx 实现
canonical_path: docs/plans/archive/022-Shared仓储基础设施边界修复计划.md
status: accepted
execution_status: done
review_status: approved
version: v1.1
owner: HotKey Server Team
inputs:
  - docs/design/archive/002-后端单体架构设计.md
  - docs/prd/archive/022-Shared仓储基础设施边界修复.md
outputs:
  - platform database repository 适配器
  - shared 基础设施中立门禁
triggers:
  - PRD-022 accepted 且 ready
downstream:
  - docs/acceptance/archive/022-Shared仓储基础设施边界修复验收.md
depends_on: [PLAN-002, PLAN-021]
---

# Shared 仓储基础设施边界修复计划

## 1. 开工门禁

PRD-022 必须为 `accepted/ready`，本 Plan 必须由非主要编写者评为 `accepted/approved/ready`，Acceptance-022 具备 RED/GREEN 表后才修改生产代码。

## 2. 文件边界

创建 `internal/platform/database/repository/{gorm,errors}.go`，移动集中测试到 `test/_suite/internal/platform/database/repository/gorm_integration_test.go`，新增架构门禁并创建 022 三层文档。

修改 `internal/shared/repository/errors.go`、29 个调用 `sharedrepository.MapError` 的模块/平台基础设施文件、`test/architecture/layout_test.go`、`AGENTS.md`、`README.md`、`README_EN.md` 及三类文档索引；删除 `internal/shared/repository/gorm.go` 和旧测试镜像路径。

明确不修改 `db/schema.sql`、`docs/openapi/`、事务实现、业务领域接口和 GoLand 配置。

## 3. 测试优先步骤

### Task 1：冻结依赖边界与错误契约

- RED：新增 `TestSharedPackagesDoNotImportInfrastructure`，扫描 shared Go import，当前 GORM/pgx/platform model 必须导致失败。
- RED：迁移测试路径并调整为 platform repository 包；扩展 `TestMapErrorUsesStableCategories` 覆盖 `gorm.ErrRecordNotFound`、全部既有 pgx code、context 取消，并用 `%w` 包装 GORM/pgx 错误冻结 `errors.Is`/`errors.As` 兼容；测试必须先因新包尚无实现失败。

### Task 2：最小迁移实现

- shared `errors.go` 只保留稳定错误哨兵；platform repository 的 `MapError` 引用这些哨兵。
- GORM CRUD/History 迁入 platform repository，方法继续使用 shared `PageQuery`/`PageResult`，不复制契约。
- 29 个生产调用文件只把 `MapError` 切换到 platform adapter，其他 shared 错误使用保持不变。

### Task 3：回归与验收

- `go test ./test/architecture -run TestSharedPackagesDoNotImportInfrastructure -count=1`
- `go run ./test/runner test ./internal/platform/database/repository -run TestMapErrorUsesStableCategories -count=1`
- `go run ./test/runner test ./internal/modules/... ./internal/platform/... -run '^$' -count=1`（编译生产包与集中测试）
- `make lint`、`make build`、`make validate`、`git diff --check`
- `git diff --exit-code -- db/schema.sql docs/openapi internal/modules/intelligence/schemas`
- 如存在可丢弃 `HOTKEY_TEST_DSN`，执行 `go run ./test/runner test ./internal/platform/database/repository -run TestGORMCRUDUsesMappedVersionSoftDeleteAndCursor -count=1`；否则记录该集成测试 blocked。
- 仅在同时存在可丢弃 `HOTKEY_TEST_DSN` 与隔离 `HOTKEY_TEST_REDIS_URL` 时执行 `make test`/`make ci`，否则按缺失依赖分别记录 blocked。
- 非主要实施者复核依赖方向、29 个调用点、错误兼容和差异范围，无 P0/P1/P2 后结案。

## 4. 安全与回滚

- 不改变或回显数据库错误内容，只改变错误映射代码所在包。
- 回滚时恢复 shared 中两个实现文件、旧测试镜像和 29 个 import/call，架构门禁同步回滚。
- 无数据迁移、Schema 回滚、服务配置或 API 回滚。

## 5. 变更记录

| 版本 | 日期 | 变更 |
|---|---|---|
| v0.1 | 2026-08-01 | 建立 shared 依赖门禁、GORM/pgx 迁移、29 个调用点与回归计划，等待独立复审。 |
| v0.2 | 2026-08-01 | 按评审补齐 GORM not-found、wrapped error 兼容、精确数据库集成命令及 PostgreSQL/Redis 双环境门禁。 |
| v1.0 | 2026-08-01 | 修订后经独立 Plan Review 通过，进入 approved/ready。 |
| v1.1 | 2026-08-01 | 依赖迁移、门禁和故障兼容测试通过独立实现复审，以 79df138 提交并归档。 |

## 6. 独立评审记录

2026-08-01，首轮评审要求补齐 GORM not-found、wrapped error、精确集成命令和 PostgreSQL/Redis 双环境门禁；v0.2 修订后复审无新的 P0/P1/P2，结论 `APPROVED`。

2026-08-01，独立实现复审提出反引号 import literal 可绕过门禁；改用 `strconv.Unquote` 并增加临时源码回归后复审通过，无新的 P0/P1/P2。
