## asynq-queue

### Requirements

1. `internal/platform/queue/client.go` MUST 提供 `NewClient(redisAddr string) *asynq.Client`
2. `internal/platform/queue/server.go` MUST 提供 `NewServer(redisAddr string, concurrency int) *asynq.Server`
3. `NewServerMux() *asynq.ServeMux` MUST 返回空 mux 供注册 handler
4. MUST 使用 `github.com/hibiken/asynq`
5. Client/Server 构造 MUST 为纯函数
6. Server MUST 支持配置 concurrency

### Scenarios

- **Success**: Client/Server 构造不 panic；mux 注册后可启动
- **Failure**: redisAddr 为空时构造仍成功（连接失败在运行时暴露）

### Validation

```bash
go build ./internal/platform/queue/...
go test ./internal/platform/queue/...
```
