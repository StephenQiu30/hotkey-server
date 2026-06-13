package queue

import (
	"github.com/hibiken/asynq"
)

// NewClient creates a new asynq client for enqueuing tasks.
// The caller is responsible for closing the client when done.
func NewClient(redisAddr string) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
}
