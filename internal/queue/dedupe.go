package queue

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const dedupKeyPrefix = "queue:dedup:"

// Dedupe provides Redis-backed message-id deduplication.
type Dedupe struct {
	client redis.UniversalClient
	ttl    time.Duration
}

func NewDedupe(client redis.UniversalClient) *Dedupe {
	return &Dedupe{client: client, ttl: 24 * time.Hour}
}

// Seen returns true if the message ID was already processed (dedup hit),
// or false if this is the first time (caller should process). On first
// encounter, atomically records the ID with TTL.
func (d *Dedupe) Seen(ctx context.Context, msgID string) (bool, error) {
	if d.client == nil {
		return false, nil // dedup disabled when no Redis configured
	}
	key := dedupKeyPrefix + msgID
	ok, err := d.client.SetNX(ctx, key, "1", d.ttl).Result()
	if err != nil {
		return false, err
	}
	return !ok, nil
}

// Mark unconditionally records the message ID as seen. Used to
// record dedup state after a handler succeeds, keeping the check
// and mark separate so retries work correctly.
func (d *Dedupe) Mark(ctx context.Context, msgID string) error {
	if d.client == nil {
		return nil
	}
	key := dedupKeyPrefix + msgID
	return d.client.Set(ctx, key, "1", d.ttl).Err()
}
