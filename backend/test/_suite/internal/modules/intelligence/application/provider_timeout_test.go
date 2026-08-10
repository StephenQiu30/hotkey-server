package application

import (
	"context"
	"testing"
	"time"
)

func TestProviderCallContextUsesConfiguredTimeout(t *testing.T) {
	started := time.Now()
	ctx, cancel := providerCallContext(context.Background(), 3)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("provider context has no deadline")
	}
	remaining := deadline.Sub(started)
	if remaining < 2900*time.Millisecond || remaining > 3100*time.Millisecond {
		t.Fatalf("provider deadline remaining = %s, want about 3s", remaining)
	}
}

func TestProviderCallContextPreservesEarlierParentDeadline(t *testing.T) {
	parent, parentCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer parentCancel()
	parentDeadline, _ := parent.Deadline()

	ctx, cancel := providerCallContext(parent, 30)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("provider context has no deadline")
	}
	if !deadline.Equal(parentDeadline) {
		t.Fatalf("provider deadline = %s, want parent deadline %s", deadline, parentDeadline)
	}
}
