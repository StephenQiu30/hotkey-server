package app

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/config"
)

type Component interface {
	Run(context.Context) error
	Shutdown(context.Context) error
}

type RuntimeComponents struct {
	API       Component
	Worker    Component
	Scheduler Component
}

type Runtime struct {
	cfg        config.Config
	components RuntimeComponents
}

func NewRuntime(cfg config.Config, components RuntimeComponents) *Runtime {
	return &Runtime{cfg: cfg, components: components}
}

func (r *Runtime) Run(ctx context.Context) error {
	selected := r.selected()
	if len(selected) == 0 {
		return nil
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errs := make(chan error, len(selected)+1)
	var wg sync.WaitGroup
	for _, component := range selected {
		wg.Add(1)
		go func(component Component) {
			defer wg.Done()
			if err := component.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
				errs <- err
				cancel()
			}
		}(component)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		cancel()
		runErr = ctx.Err()
	case runErr = <-errs:
		cancel()
	case <-done:
		cancel()
	}

	if runErr != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		runErr = errors.Join(runErr, r.Shutdown(shutdownCtx))
		<-done
	}
	close(errs)

	for err := range errs {
		if err != nil {
			runErr = errors.Join(runErr, err)
		}
	}
	return runErr
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	var joined error
	for _, component := range r.selected() {
		if err := component.Shutdown(ctx); err != nil {
			joined = errors.Join(joined, err)
		}
	}
	return joined
}

func (r *Runtime) selected() []Component {
	var components []Component
	switch r.cfg.RuntimeMode {
	case config.RuntimeModeAPI:
		components = appendIfPresent(components, r.components.API)
	case config.RuntimeModeWorker:
		components = appendIfPresent(components, r.components.Worker, r.components.Scheduler)
	default:
		components = appendIfPresent(components, r.components.API, r.components.Worker, r.components.Scheduler)
	}
	return components
}

func appendIfPresent(components []Component, candidates ...Component) []Component {
	for _, candidate := range candidates {
		if candidate != nil {
			components = append(components, candidate)
		}
	}
	return components
}
