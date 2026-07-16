package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	intelligencepostgres "github.com/StephenQiu30/hotkey-server/internal/modules/intelligence/infrastructure/postgres"
	sharedclock "github.com/StephenQiu30/hotkey-server/internal/shared/clock"
	"go.uber.org/fx"
)

// RunLeaseReclaimer only marks crashed in-flight runs terminal and releases
// their reservation. It never invokes a provider or attempts to replay work.
type RunLeaseReclaimer struct {
	runs  *intelligencepostgres.Repository
	clock sharedclock.Clock

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewRunLeaseReclaimer(runs *intelligencepostgres.Repository) (*RunLeaseReclaimer, error) {
	if runs == nil {
		return nil, fmt.Errorf("AI run repository is required")
	}
	return &RunLeaseReclaimer{runs: runs, clock: sharedclock.System{}}, nil
}

// RegisterRunLeaseReclaimerLifecycle is invoked only by worker-capable Fx
// roles in bootstrap. A short ticker keeps recovery bounded without replaying
// network work after process failure.
func RegisterRunLeaseReclaimerLifecycle(lifecycle fx.Lifecycle, reclaimer *RunLeaseReclaimer) {
	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			reclaimer.start()
			return nil
		},
		OnStop: func(context.Context) error {
			return reclaimer.stop()
		},
	})
}

func (reclaimer *RunLeaseReclaimer) start() {
	reclaimer.mu.Lock()
	defer reclaimer.mu.Unlock()
	if reclaimer.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	reclaimer.cancel = cancel
	reclaimer.done = make(chan struct{})
	done := reclaimer.done
	go func(done chan struct{}) {
		defer close(done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = reclaimer.runs.ReclaimExpired(ctx, reclaimer.clock.Now())
			}
		}
	}(done)
}

func (reclaimer *RunLeaseReclaimer) stop() error {
	reclaimer.mu.Lock()
	cancel, done := reclaimer.cancel, reclaimer.done
	reclaimer.cancel, reclaimer.done = nil, nil
	reclaimer.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-done
	return nil
}
