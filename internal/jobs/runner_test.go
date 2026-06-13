package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerExecutesJobAtLeastOnce(t *testing.T) {
	var count atomic.Int32
	r := NewRunner()
	r.Register("test-job", func(ctx context.Context) error {
		count.Add(1)
		return nil
	}, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	r.Run(ctx)

	if count.Load() < 1 {
		t.Fatal("expected job to execute at least once")
	}
}

func TestRunnerStopsOnContextCancel(t *testing.T) {
	var count atomic.Int32
	r := NewRunner()
	r.Register("cancel-job", func(ctx context.Context) error {
		count.Add(1)
		return nil
	}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("runner did not stop after context cancel")
	}

	final := count.Load()
	if final < 1 {
		t.Fatal("expected at least one execution before stop")
	}
}

func TestRunnerHandlesJobError(t *testing.T) {
	var count atomic.Int32
	r := NewRunner()
	r.Register("error-job", func(ctx context.Context) error {
		count.Add(1)
		return context.DeadlineExceeded
	}, 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	r.Run(ctx)

	if count.Load() < 1 {
		t.Fatal("expected job to run despite errors")
	}
}
