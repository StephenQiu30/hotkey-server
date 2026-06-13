package queue

import (
	"github.com/hibiken/asynq"
)

// NewServer creates a new asynq server with the given concurrency.
func NewServer(redisAddr string, concurrency int) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{Concurrency: concurrency},
	)
}

// NewServeMux creates a new asynq ServeMux for registering task handlers.
func NewServeMux() *asynq.ServeMux {
	return asynq.NewServeMux()
}
