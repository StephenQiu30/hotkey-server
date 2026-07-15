package bootstrap

import (
	"context"
	"flag"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/internal/platform/database"
)

func runDatabaseCommand(ctx context.Context, cfg config.Config, args []string) error {
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate database command configuration: %w", err)
	}
	if len(args) == 0 {
		return fmt.Errorf("database command is required: expected init or verify")
	}

	runtime, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()

	switch args[0] {
	case "init":
		return runDatabaseInit(ctx, runtime, args[1:])
	case "verify":
		if len(args) != 1 {
			return fmt.Errorf("db verify does not accept arguments")
		}
		verification, err := database.Verify(ctx, runtime.Pool)
		if err != nil {
			return err
		}
		fmt.Printf("database verified: PostgreSQL=%d tables=%d fingerprint=%s\n", verification.ServerVersion, len(verification.Tables), verification.CatalogFingerprint)
		return nil
	default:
		return fmt.Errorf("unknown database command %q: expected init or verify", args[0])
	}
}

func runDatabaseInit(ctx context.Context, runtime *database.Runtime, args []string) error {
	flags := flag.NewFlagSet("hotkey db init", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	emptyOnly := flags.Bool("empty-only", false, "require an empty public schema")
	confirmed := flags.Bool("confirm-empty", false, "confirm initialization of the configured empty database")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse db init flags: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected db init arguments: %v", flags.Args())
	}
	if !*emptyOnly || !*confirmed {
		return fmt.Errorf("db init requires --empty-only --confirm-empty")
	}
	if err := database.InitializeEmpty(ctx, runtime.Pool); err != nil {
		return err
	}
	verification, err := database.Verify(ctx, runtime.Pool)
	if err != nil {
		return fmt.Errorf("verify initialized database: %w", err)
	}
	fmt.Printf("database initialized: PostgreSQL=%d tables=%d fingerprint=%s\n", verification.ServerVersion, len(verification.Tables), verification.CatalogFingerprint)
	return nil
}
