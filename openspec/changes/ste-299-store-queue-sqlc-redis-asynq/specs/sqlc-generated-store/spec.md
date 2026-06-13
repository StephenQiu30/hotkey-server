## sqlc-generated-store

### Requirements

1. `sqlc.yaml` MUST 配置 `engine: postgresql`, `pgx/v5` driver, 输出到 `internal/database/sqlc/`, package 名 `sqlc`
2. `db/queries/users.sql` MUST 包含: `GetUserByEmail`, `GetUserByID`, `CreateUser`, `ExistsByEmail`
3. `db/queries/keyword_monitors.sql` MUST 包含: `CreateMonitor`, `GetMonitorByID`, `ListMonitorsByUser`, `UpdateMonitor`
4. `db/queries/user_notifications.sql` MUST 包含: `CreateNotification`, `ListUnreadNotifications`, `MarkNotificationRead`
5. 生成代码 MUST 可编译（`go build ./internal/database/sqlc/...`）
6. 每个 query MUST 使用 `sqlc.arg()` 参数语法（非 `$1` 占位符）
7. `:one` / `:many` / `:exec` 注解 MUST 与返回类型匹配

### Scenarios

- **Success**: `sqlc generate` 产出可编译的 Go 文件；`go vet` 无错误
- **Failure**: SQL 语法错误导致 generate 失败；生成代码缺少 import 导致编译失败

### Validation

```bash
sqlc generate
go build ./internal/database/sqlc/...
go vet ./internal/database/sqlc/...
```
