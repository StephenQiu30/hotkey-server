// Package testutil provides shared test helpers for setting up test infrastructure
// such as databases and Redis connections.
package testutil

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
)

// NewTestRedis creates a *redis.Client pointing at TEST_REDIS_ADDR (falling back
// to REDIS_ADDR then "localhost:6379"). If the target is not reachable the test
// is marked as skipped so that CI / local runs without a running Redis instance
// pass gracefully.
func NewTestRedis(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		addr = os.Getenv("REDIS_ADDR")
	}
	if addr == "" {
		addr = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{Addr: addr})

	// Probe the connection.
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Logf("redis not reachable at %s: %v — skipping test", addr, err)
		t.Skip("SKIP: no reachable Redis")
	}

	return rdb
}

// FlushTestRedis flushes all keys in the currently selected Redis database.
// It is a no-op if rdb is nil (e.g. when the test was skipped).
func FlushTestRedis(t *testing.T, rdb *redis.Client) {
	t.Helper()
	if rdb == nil {
		return
	}
	if err := rdb.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("failed to flush test redis: %v", err)
	}
}

// CleanupTestRedis closes the Redis connection. It is a no-op if rdb is nil.
// An error from closing an already-closed client is silently ignored.
func CleanupTestRedis(t *testing.T, rdb *redis.Client) {
	t.Helper()
	if rdb == nil {
		return
	}
	if err := rdb.Close(); err != nil && !errors.Is(err, redis.ErrClosed) {
		t.Errorf("failed to close test redis: %v", err)
	}
}
