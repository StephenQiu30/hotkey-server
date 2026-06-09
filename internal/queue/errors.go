package queue

import "errors"

// RedisConnectionError 表示 Redis 连接层失败（连接拒绝、超时等）。
// Worker 应区分此错误与 ErrNoJobs，避免静默丢弃任务。
type RedisConnectionError struct {
	err error
}

func NewRedisConnectionError(err error) error {
	if err == nil {
		return nil
	}
	return RedisConnectionError{err: err}
}

func (e RedisConnectionError) Error() string {
	if e.err == nil {
		return "redis connection error"
	}
	return "redis connection error: " + e.err.Error()
}

func (e RedisConnectionError) Unwrap() error {
	return e.err
}

// IsRedisConnectionError 检查错误链中是否包含 RedisConnectionError。
func IsRedisConnectionError(err error) bool {
	var connErr RedisConnectionError
	return errors.As(err, &connErr)
}
