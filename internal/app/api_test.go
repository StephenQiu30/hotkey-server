package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/StephenQiu30/hotkey-server/internal/config"
)

func TestNewAPIInjectsScoringService(t *testing.T) {
	// NewAPI 应当接受 ScoringService 并将其注入到 router 的 Dependencies 中。
	// 传入 nil 时 router 会回退到内存实现，但 wiring 路径必须存在。
	api := NewAPI(config.Config{
		HTTPAddr:        "127.0.0.1:0",
		AuthTokenSecret: "test-secret",
		EncryptionKey:   "0123456789abcdef0123456789abcdef", // 32 bytes for AES-256
	}, slog.Default(), nil, nil, nil)
	if api == nil {
		t.Fatal("expected non-nil API from NewAPI")
	}
}

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
