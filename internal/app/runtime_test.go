package app

import (
	"context"
	"errors"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/config"
)

type runtimeComponent struct {
	starts    int
	shutdowns int
}

type failingRuntimeComponent struct {
	err error
}

func (c *failingRuntimeComponent) Run(context.Context) error {
	return c.err
}

func (c *failingRuntimeComponent) Shutdown(context.Context) error {
	return nil
}

func TestRuntimeRunJoinsComponentErrors(t *testing.T) {
	errA := errors.New("api failed")
	errB := errors.New("worker failed")
	runtime := NewRuntime(config.Config{RuntimeMode: config.RuntimeModeAll}, RuntimeComponents{
		API:       &failingRuntimeComponent{err: errA},
		Worker:    &failingRuntimeComponent{err: errB},
		Scheduler: &runtimeComponent{},
	})

	err := runtime.Run(context.Background())
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("expected joined errors containing %v and %v, got %v", errA, errB, err)
	}
}

func (c *runtimeComponent) Run(context.Context) error {
	c.starts++
	return nil
}

func (c *runtimeComponent) Shutdown(context.Context) error {
	c.shutdowns++
	return nil
}

func TestRuntimeModeStartsExpectedComponents(t *testing.T) {
	tests := []struct {
		name       string
		mode       config.RuntimeMode
		wantAPI    int
		wantWorker int
		wantSched  int
	}{
		{"api only", config.RuntimeModeAPI, 1, 0, 0},
		{"worker only", config.RuntimeModeWorker, 0, 1, 1},
		{"all", config.RuntimeModeAll, 1, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &runtimeComponent{}
			worker := &runtimeComponent{}
			scheduler := &runtimeComponent{}
			runtime := NewRuntime(config.Config{RuntimeMode: tt.mode}, RuntimeComponents{
				API:       api,
				Worker:    worker,
				Scheduler: scheduler,
			})

			if err := runtime.Run(context.Background()); err != nil {
				t.Fatalf("run failed: %v", err)
			}
			if api.starts != tt.wantAPI || worker.starts != tt.wantWorker || scheduler.starts != tt.wantSched {
				t.Fatalf("starts api=%d worker=%d scheduler=%d, want api=%d worker=%d scheduler=%d", api.starts, worker.starts, scheduler.starts, tt.wantAPI, tt.wantWorker, tt.wantSched)
			}
		})
	}
}
