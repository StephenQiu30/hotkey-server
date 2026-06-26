package app

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestShutdownHonorsContextWhileWaitingForWorker(t *testing.T) {
	a := &App{
		server: &http.Server{},
		cancel: func() {},
	}
	a.wg.Add(1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	err := a.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected shutdown deadline while waiting for worker, got %v", err)
	}
}

func TestShutdownReturnsAfterWorkerStops(t *testing.T) {
	a := &App{
		server: &http.Server{},
		cancel: func() {},
	}
	a.wg = sync.WaitGroup{}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := a.Shutdown(ctx); err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}
}
