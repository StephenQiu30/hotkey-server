## Why

hotkey-server 当前使用手写 SQL（`database/sql` + raw query）访问 PostgreSQL，无 Redis 客户端，无任务队列。ADR-010~012 选型已冻结（sqlc / go-redis / asynq），需要落地基础设施层，为 L4 业务闭环提供编译通过的 store + queue 骨架。

## What Changes

- 新增 `sqlc.yaml` 配置 + `db/queries/*.sql` 覆盖 auth/monitor/notify 三域核心查询
- sqlc 生成代码输出到 `internal/database/sqlc/`
- 新增 `internal/platform/redis/client.go`：go-redis/v9 连接 + health check
- 新增 `internal/platform/queue/client.go`：asynq client（enqueue 骨架）
- 新增 `internal/platform/queue/server.go`：asynq server wiring
- `go.mod` 新增 `redis/go-redis/v9` + `hibiken/asynq` 依赖
- 新增单元测试覆盖 redis health、queue client/server 构造

## Capabilities

### New Capabilities

- `sqlc-generated-store`: sqlc 配置、SQL 查询定义、生成的 Go 代码，覆盖 users/keyword_monitors/user_notifications 表
- `redis-client`: go-redis/v9 连接管理、健康检查
- `asynq-queue`: asynq client enqueue 骨架 + server handler 注册 wiring

### Modified Capabilities

（无现有 spec 需修改）

## Impact

- 新增文件：`sqlc.yaml`, `db/queries/*.sql`, `internal/database/sqlc/`, `internal/platform/redis/`, `internal/platform/queue/`
- 依赖变更：`go.mod` 新增 redis/go-redis/v9, hibiken/asynq, sqlc-dev/sqlc
- 不修改现有 `internal/database/*.go`（Repository 迁移属 L4）
- 不修改 `cmd/api/main.go`（Fx/Wire wiring 属 L1）
