package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"
)

func TestAPIRunStopsOnContextCancel(t *testing.T) {
	api := &API{
		server: &http.Server{
			Addr:    "127.0.0.1:0",
			Handler: http.NewServeMux(),
		},
		logger: slog.Default(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- api.Run(ctx)
	}()

	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
