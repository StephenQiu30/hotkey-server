package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache is a generic Redis cache using the Cache-Aside pattern.
type Cache[T any] struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

// NewCache creates a new generic Cache with the given key prefix and TTL.
func NewCache[T any](client *redis.Client, prefix string, ttl time.Duration) *Cache[T] {
	return &Cache[T]{client: client, prefix: prefix, ttl: ttl}
}

func (c *Cache[T]) key(id string) string { return c.prefix + ":" + id }

// Get retrieves a cached value by key. Returns zero value, false if cache miss.
func (c *Cache[T]) Get(ctx context.Context, id string) (T, bool, error) {
	val, err := c.client.Get(ctx, c.key(id)).Result()
	if errors.Is(err, redis.Nil) {
		var zero T
		return zero, false, nil
	}
	if err != nil {
		var zero T
		return zero, false, err
	}
	var result T
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		var zero T
		return zero, false, err
	}
	return result, true, nil
}

// Set stores a value in the cache with the configured TTL.
func (c *Cache[T]) Set(ctx context.Context, id string, val T) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.key(id), data, c.ttl).Err()
}

// Del removes a key from the cache.
func (c *Cache[T]) Del(ctx context.Context, id string) error {
	return c.client.Del(ctx, c.key(id)).Err()
}
