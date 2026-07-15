package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"go.uber.org/zap"
)

func TestApplicationRolesStartAndStopIndependently(t *testing.T) {
	t.Parallel()

	for _, role := range []Role{RoleAll, RoleAPI, RoleWorker} {
		role := role
		t.Run(string(role), func(t *testing.T) {
			t.Parallel()

			cfg := config.Default()
			cfg.Role = string(role)
			if role.StartsAPI() {
				cfg.HTTPAddr = "127.0.0.1:0"
			} else {
				cfg.HTTPAddr = ""
			}

			app, err := NewApp(cfg, zap.NewNop())
			if err != nil {
				t.Fatalf("NewApp() error = %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := app.Start(ctx); err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			if err := app.Stop(ctx); err != nil {
				t.Fatalf("Stop() error = %v", err)
			}
		})
	}
}

func TestNewAppRejectsInvalidRole(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Role = "scheduler"
	if _, err := NewApp(cfg, zap.NewNop()); err == nil {
		t.Fatal("NewApp() error = nil, want an error")
	}
}
