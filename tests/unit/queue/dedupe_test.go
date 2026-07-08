package queue_test

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/queue"
)

func TestDedupeReturnsFalseForNilRedis(t *testing.T) {
	t.Parallel()

	d := queue.NewDedupe(nil)
	seen, err := d.Seen(context.Background(), "msg-1")
	if err != nil {
		t.Fatalf("Seen returned error: %v", err)
	}
	if seen {
		t.Fatal("Seen = true without Redis, want false (dedup disabled)")
	}
}
