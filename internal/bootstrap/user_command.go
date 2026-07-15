package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/internal/modules/identity/infrastructure/security"
	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

var ErrBootstrapAdminUnavailable = errors.New("bootstrap admin is unavailable after users exist")

func runUserCommand(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("user command is required: expected bootstrap-admin")
	}
	if args[0] != "bootstrap-admin" {
		return fmt.Errorf("unknown user command %q: expected bootstrap-admin", args[0])
	}
	if len(args) != 1 {
		return fmt.Errorf("user bootstrap-admin does not accept arguments")
	}
	if strings.TrimSpace(cfg.Authentication.BootstrapAdminEmail) == "" || strings.TrimSpace(cfg.Authentication.BootstrapAdminPassword) == "" {
		return fmt.Errorf("HOTKEY_BOOTSTRAP_ADMIN_EMAIL and HOTKEY_BOOTSTRAP_ADMIN_PASSWORD are required")
	}
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate bootstrap-admin configuration: %w", err)
	}
	runtime, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	if _, err := database.Verify(ctx, runtime.Pool); err != nil {
		return fmt.Errorf("verify database compatibility: %w", err)
	}
	return runBootstrapAdmin(ctx, runtime, cfg.Authentication)
}

func runBootstrapAdmin(ctx context.Context, runtime *database.Runtime, authentication config.AuthenticationConfig) error {
	if strings.TrimSpace(authentication.BootstrapAdminEmail) == "" || strings.TrimSpace(authentication.BootstrapAdminPassword) == "" {
		return fmt.Errorf("HOTKEY_BOOTSTRAP_ADMIN_EMAIL and HOTKEY_BOOTSTRAP_ADMIN_PASSWORD are required")
	}
	hasher := security.NewPasswordHasher()
	passwordHash, err := hasher.Hash(authentication.BootstrapAdminPassword)
	if err != nil {
		return fmt.Errorf("hash bootstrap administrator password: %w", err)
	}
	_, err = postgres.NewUserRepository(runtime).BootstrapAdmin(ctx, authentication.BootstrapAdminEmail, passwordHash)
	if errors.Is(err, postgres.ErrBootstrapUnavailable) {
		return ErrBootstrapAdminUnavailable
	}
	return err
}
