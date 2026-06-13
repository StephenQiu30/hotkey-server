## Tasks

### T1: sqlc 基础设施（ADR-010）

- [x] T1.1 创建 `sqlc.yaml`（engine: postgresql, pgx/v5, output: internal/database/sqlc, package: sqlc）
- [x] T1.2 创建 `db/queries/users.sql`（GetUserByEmail, GetUserByID, CreateUser, ExistsByEmail）
- [x] T1.3 创建 `db/queries/keyword_monitors.sql`（CreateMonitor, GetMonitorByID, ListMonitorsByUser, UpdateMonitor）
- [x] T1.4 创建 `db/queries/user_notifications.sql`（CreateNotification, ListUnreadNotifications, MarkNotificationRead）
- [x] T1.5 `sqlc generate` 生成代码
- [x] T1.6 `go build ./internal/database/sqlc/...` 编译验证

**Validation:** `sqlc generate && go build ./internal/database/sqlc/...`

### T2: go-redis 客户端（ADR-011）

- [x] T2.1 `go get github.com/redis/go-redis/v9`
- [x] T2.2 创建 `internal/platform/redis/client.go`（NewClient + HealthCheck）
- [x] T2.3 创建 `internal/platform/redis/client_test.go`

**Validation:** `go build ./internal/platform/redis/... && go test ./internal/platform/redis/...`

### T3: asynq 队列 wiring（ADR-012）

- [x] T3.1 `go get github.com/hibiken/asynq`
- [x] T3.2 创建 `internal/platform/queue/client.go`（NewClient）
- [x] T3.3 创建 `internal/platform/queue/server.go`（NewServer + NewServerMux）
- [x] T3.4 创建 `internal/platform/queue/queue_test.go`

**Validation:** `go build ./internal/platform/queue/... && go test ./internal/platform/queue/...`

### T4: 集成验证

- [x] T4.1 `go mod tidy`
- [x] T4.2 `go build ./...`
- [x] T4.3 `go vet ./...`
- [x] T4.4 `make validate`

**Validation:** `go build ./... && go vet ./... && make validate`
