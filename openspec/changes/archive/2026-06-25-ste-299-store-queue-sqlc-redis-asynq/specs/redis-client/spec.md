## redis-client

### Requirements

1. `internal/platform/redis/client.go` MUST 提供 `NewClient(addr string) *redis.Client`
2. `HealthCheck(ctx, client) error` MUST 执行 `PING` 并返回错误
3. MUST 使用 `github.com/redis/go-redis/v9`
4. Client 构造 MUST 为纯函数，不持有全局状态
5. HealthCheck MUST 接受 `context.Context` 以支持超时

### Scenarios

- **Success**: 连接可达时 HealthCheck 返回 nil
- **Failure**: 连接不可达时 HealthCheck 返回非 nil error

### Validation

```bash
go build ./internal/platform/redis/...
go test ./internal/platform/redis/...
```
