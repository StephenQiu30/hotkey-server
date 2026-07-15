package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/domain"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/tests/postgresfixture"
)

func TestBootstrapAdminCommandUsesOnlyBootstrapConfigurationAndEmptyUserTable(t *testing.T) {
	runtime := newBootstrapRuntime(t)
	cfg := config.Default()
	cfg.Authentication.BootstrapAdminEmail = "bootstrap@example.test"
	cfg.Authentication.BootstrapAdminPassword = "correct horse battery staple"

	if err := runBootstrapAdmin(context.Background(), runtime, cfg.Authentication); err != nil {
		t.Fatalf("runBootstrapAdmin(): %v", err)
	}
	var role, status string
	if err := runtime.SQL.QueryRow(`SELECT role, status FROM users WHERE email = $1`, "bootstrap@example.test").Scan(&role, &status); err != nil {
		t.Fatalf("read bootstrap user: %v", err)
	}
	if role != string(domain.RoleAdmin) || status != string(domain.UserStatusActive) {
		t.Fatalf("bootstrap user = role %q status %q, want active admin", role, status)
	}
	if err := runBootstrapAdmin(context.Background(), runtime, cfg.Authentication); !errors.Is(err, ErrBootstrapAdminUnavailable) {
		t.Fatalf("second runBootstrapAdmin() error = %v, want ErrBootstrapAdminUnavailable", err)
	}
}

func TestUserCommandRejectsArgumentsAndMissingBootstrapInputs(t *testing.T) {
	cfg := config.Default()
	if err := runUserCommand(context.Background(), cfg, []string{"bootstrap-admin", "--email", "ignored@example.test"}); err == nil {
		t.Fatal("runUserCommand() error = nil, want rejected arguments")
	}
	if err := runUserCommand(context.Background(), cfg, []string{"bootstrap-admin"}); err == nil {
		t.Fatal("runUserCommand() error = nil, want missing bootstrap inputs")
	}
}

func newBootstrapRuntime(t *testing.T) *database.Runtime {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	runtime, err := database.Open(ctx, postgresfixture.New(t))
	if err != nil {
		t.Fatalf("database.Open(): %v", err)
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		_ = runtime.Close()
		t.Fatalf("database.InitializeEmpty(): %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}
