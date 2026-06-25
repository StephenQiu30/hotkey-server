## Design

### Goals

- 落地 ADR-010（sqlc）、ADR-011（go-redis）、ADR-012（asynq）基础设施
- 提供编译通过的 store + queue 骨架，L4 可直接集成
- 保持与现有 `internal/database/*.go` 手写 SQL 并存（渐进迁移）

### Non-Goals

- 不迁移现有 Repository 到 sqlc（L4 范围）
- 不在 `cmd/api/main.go` 做 Fx/Wire wiring（L1 范围）
- 不实现业务 job handler（L4 范围）

### Contracts

**sqlc 生成代码接口：**

```go
// internal/database/sqlc/querier.go (generated)
type Querier interface {
    GetUserByEmail(ctx context.Context, email string) (User, error)
    GetUserByID(ctx context.Context, id int64) (User, error)
    CreateUser(ctx context.Context, arg CreateUserParams) (User, error)
    ExistsByEmail(ctx context.Context, email string) (bool, error)
    CreateMonitor(ctx context.Context, arg CreateMonitorParams) (KeywordMonitor, error)
    GetMonitorByID(ctx context.Context, id int64) (KeywordMonitor, error)
    ListMonitorsByUser(ctx context.Context, userID int64) ([]KeywordMonitor, error)
    UpdateMonitor(ctx context.Context, arg UpdateMonitorParams) (KeywordMonitor, error)
    CreateNotification(ctx context.Context, arg CreateNotificationParams) (UserNotification, error)
    ListUnreadNotifications(ctx context.Context, userID int64) ([]UserNotification, error)
    MarkNotificationRead(ctx context.Context, id int64) error
}
```

**Redis 接口：**

```go
func NewClient(addr string) *redis.Client
func HealthCheck(ctx context.Context, client *redis.Client) error
```

**Queue 接口：**

```go
func NewClient(redisAddr string) *asynq.Client
func NewServer(redisAddr string, concurrency int) *asynq.Server
func NewServerMux() *asynq.ServeMux
```

### State Flow

```
db/queries/*.sql  →  sqlc generate  →  internal/database/sqlc/*.go
                                           ↓
                                    Repository (L4) 调用 sqlc.Querier

redis addr  →  platform/redis.NewClient  →  HealthCheck
                                            ↓
                                     Repository/Cache (L4)

redis addr  →  platform/queue.NewClient  →  Enqueue (L4)
             platform/queue.NewServer  →  RegisterHandlers → Run (L4)
```

### Failure Paths

- sqlc generate 失败：SQL 语法错误 → 修复 query 重新 generate
- Redis 连接失败：HealthCheck 返回 error → 上层决定降级或启动失败
- asynq 连接失败：Server.Run 返回 error → Fx lifecycle OnStop 处理

### Rollback

- 删除 `sqlc.yaml`, `db/queries/`, `internal/database/sqlc/`, `internal/platform/redis/`, `internal/platform/queue/`
- `go mod tidy` 移除新增依赖
- 现有手写 SQL 代码不受影响
