package redis

import (
	"context"
	"testing"
	"time"
)

func TestClientUnavailableReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := NewClient("redis://127.0.0.1:1/0", Options{DialTimeout: 10 * time.Millisecond})
	if err := client.Ping(ctx); err == nil {
		t.Fatal("expected unavailable redis to return an error")
	}
}
