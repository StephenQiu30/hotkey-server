package app

import (
	"context"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/config"
)

type runtimeComponent struct {
	starts    int
	shutdowns int
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
