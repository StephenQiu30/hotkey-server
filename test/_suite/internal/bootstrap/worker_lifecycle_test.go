package bootstrap

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type lifecycleWorkerFake struct {
	mu            sync.Mutex
	reclaimCalls  int
	runCalls      int
	runStarted    chan struct{}
	workerStopped chan struct{}
	stopOnce      sync.Once
}

func (fake *lifecycleWorkerFake) ReclaimStale(context.Context, time.Duration) (int64, error) {
	fake.mu.Lock()
	fake.reclaimCalls++
	fake.mu.Unlock()
	return 0, nil
}

func (fake *lifecycleWorkerFake) Run(ctx context.Context, _ time.Duration) error {
	fake.mu.Lock()
	fake.runCalls++
	fake.mu.Unlock()
	select {
	case fake.runStarted <- struct{}{}:
	default:
	}
	<-ctx.Done()
	fake.stopOnce.Do(func() { close(fake.workerStopped) })
	return ctx.Err()
}

func TestPersistentWorkerLifecycleReclaimsStartsAndStopsWorkers(t *testing.T) {
	fake := &lifecycleWorkerFake{runStarted: make(chan struct{}, 2), workerStopped: make(chan struct{})}
	cfg := config.Default()
	cfg.Role = string(RoleWorker)
	cfg.WorkerConcurrency = 2
	cfg.WorkerPollInterval = time.Millisecond
	cfg.WorkerLeaseTimeout = time.Second
	app := fx.New(
		fx.Supply(cfg, zap.NewNop()),
		fx.Provide(func() workerRunner { return fake }),
		fx.Invoke(registerPersistentWorkerLifecycle),
	)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-fake.runStarted:
	case <-ctx.Done():
		t.Fatal("worker loop did not start")
	}
	select {
	case <-fake.runStarted:
	case <-ctx.Done():
		t.Fatal("worker concurrency was not started")
	}
	if err := app.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	select {
	case <-fake.workerStopped:
	case <-ctx.Done():
		t.Fatal("worker loop did not stop")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.reclaimCalls != 1 || fake.runCalls != 2 {
		t.Fatalf("worker lifecycle calls = reclaim %d/run %d, want 1/2", fake.reclaimCalls, fake.runCalls)
	}
}
