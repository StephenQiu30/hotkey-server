---
layer: Plan
doc_no: 016
audience: Dev, QA, Ops
feature_area: worker收敛与遗留清理
purpose: 定义 worker 运行时收敛、后台任务迁移和旧主线删除清单
canonical_path: docs/plans/016-worker与遗留清理计划.md
status: completed
version: v1.1
owner: Codex
inputs:
  - docs/design/005-平台整体重构设计.md
  - docs/design/006-Go后端工程与启动架构设计.md
  - docs/design/007-核心数据模型与数据库设计.md
  - docs/plans/013-应用骨架与启动收敛计划.md
  - docs/plans/014-Gin-HTTP迁移计划.md
  - docs/plans/015-GORM与数据库迁移计划.md
outputs:
  - worker 新运行时边界
  - 旧路由与旧持久化删除清单
  - 最终单一路径验收基线
triggers:
  - HTTP 与 ORM 主线迁移完成后
downstream: []
---

# 背景

在新主线中，worker 不再是另一套系统，而是与 API 共用单入口、单进程、单生命周期的后台运行时。为了真正结束迁移，必须单独处理 worker 收敛和遗留清理。

# 目标

1. 收敛 worker 启动、调度、退出与依赖注入边界。
2. 确定后台任务的新运行时结构。
3. 删除或降级旧路由、旧数据库主线、旧脚本叙事和无效依赖。
4. 形成最终“单主线、单入口、单部署叙事”的验收基线。

# 非目标

1. 本计划不再重新设计新的业务能力。
2. 本计划不引入新的基础设施平台。

# Task 1: 收敛 worker 运行时

目标：

1. worker 运行时由 `internal/worker` 或 app 统一装配点管理。
2. API 与 worker 使用统一退出信号。
3. 单进程内后台任务的启动和关闭流程可验证。

验证门禁：

```bash
go test ./internal/worker ./internal/app ./...
```

# Task 2: 收敛后台任务依赖与运行模式

目标：

1. 后台任务不复制独立业务层。
2. 任务统一通过 service / repository / platform 边界工作。
3. 明确 Redis、数据库和通知能力在 worker 侧的正式使用方式。

验证门禁：

```bash
go test ./internal/jobs ./tests/integration/... ./...
```

# Task 3: 清理旧主线与无效依赖

清理目标包括：

1. 旧 `Huma` 路由主线
2. 旧 `internal/server` 路由层
3. 旧 raw SQL / 旧生成主线
4. 未继续采用的复杂工程依赖
5. 冗余正式启动脚本叙事

验证门禁：

```bash
go mod tidy
rg -n "huma|internal/server|fx|wire|cobra|sqlc" internal cmd scripts
```

说明：

1. 历史文档中的旧技术引用允许保留，用于追溯，不纳入本阶段代码清理失败条件。
2. 清理门禁面向运行代码、入口与脚本，不面向历史方案文档。

# Task 4: 最终单一路径验收

目标：

1. 唯一入口、唯一 HTTP 主线、唯一 ORM 主线成立。
2. README、设计文档、计划文档、验证脚本和 Docker 叙事一致。
3. 单副本 API + worker + PostgreSQL + Redis 可跑通。

验证门禁：

```bash
go test ./...
go build ./...
docker compose config
bash scripts/smoke-api.sh
```

# 风险与边界

1. 如果遗留清理不单独拆阶段，迁移完成后仍会残留“能跑但不清晰”的技术债。
2. 如果 worker 运行边界不先收敛，多副本与幂等问题会被继续掩盖。

# 变更记录

## v1.1（2026-06-25 完成）

1. Task 3 遗留清理已完成：Huma、internal/server、sqlc、redis/queue 占位、域 http.go 已删除。
2. Worker 持久化已迁入 `internal/database` GORM，消除 API/Worker 双轨。
3. 验证门禁：`go test ./...`、`rg` 无 huma/internal/server/sqlc 运行代码引用。

## v1.0

1. 新建 worker 收敛与遗留清理计划。
2. 将后台任务收敛、旧主线删除和最终单一路径验收合并为收尾阶段。
