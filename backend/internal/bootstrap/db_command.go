package bootstrap

import (
	"context"
	"flag"
	"fmt"

	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func runDatabaseCommand(ctx context.Context, cfg config.Config, args []string) error {
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate database command configuration: %w", err)
	}
	if len(args) == 0 {
		return fmt.Errorf("database command is required: expected init, verify, or upgrade")
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
	case "upgrade":
		return runDatabaseUpgrade(ctx, runtime, args[1:])
	default:
		return fmt.Errorf("unknown database command %q: expected init, verify, or upgrade", args[0])
	}
}

func runDatabaseUpgrade(ctx context.Context, runtime *database.Runtime, args []string) error {
	flags := flag.NewFlagSet("hotkey db upgrade", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	target := flags.String("target", "", "canonical upgrade target")
	dryRun := flags.Bool("dry-run", false, "inspect without changing the database")
	applySchema := flags.Bool("apply-schema", false, "apply the embedded canonical schema")
	confirmNonEmpty := flags.Bool("confirm-non-empty", false, "confirm upgrade of a non-empty schema")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse db upgrade flags: %w", err)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected db upgrade arguments: %v", flags.Args())
	}
	if *target != database.CanonicalUpgradeTarget {
		return fmt.Errorf("db upgrade requires --target %s", database.CanonicalUpgradeTarget)
	}
	if *dryRun == *applySchema {
		return fmt.Errorf("db upgrade requires exactly one of --dry-run or --apply-schema")
	}
	if *dryRun {
		if *confirmNonEmpty {
			return fmt.Errorf("db upgrade --dry-run does not accept --confirm-non-empty")
		}
		inspection, err := database.InspectCanonicalUpgrade(ctx, runtime.Pool, *target)
		if err != nil {
			return err
		}
		fmt.Printf(
			"database upgrade dry-run: target=%s PostgreSQL=%d current_fingerprint=%s target_schema_sha256=%s current_tables=%d target_tables=%d missing=%v unexpected=%v missing_indexes=%d estimated_rows=%d table_bytes=%d index_bytes=%d estimated_index_workspace_bytes=%d estimate_version=%s lock_risk=%s extensions=%v blockers=%v\n",
			inspection.Target, inspection.ServerVersion, inspection.CurrentCatalogFingerprint,
			inspection.TargetSchemaSHA256, len(inspection.CurrentTables), len(inspection.TargetTables),
			inspection.MissingTables, inspection.UnexpectedTables, inspection.MissingIndexCount,
			inspection.EstimatedRows, inspection.CurrentTableBytes, inspection.CurrentIndexBytes,
			inspection.EstimatedIndexWorkspaceBytes, inspection.IndexEstimateVersion, inspection.LockRisk,
			inspection.Extensions, inspection.Blockers,
		)
		if !inspection.CanApply() {
			return fmt.Errorf("database upgrade dry-run found blockers: %v", inspection.Blockers)
		}
		return nil
	}
	if !*confirmNonEmpty {
		return fmt.Errorf("db upgrade --apply-schema requires --confirm-non-empty")
	}
	verification, err := database.ApplyCanonicalUpgrade(ctx, runtime.Pool, *target)
	if err != nil {
		return err
	}
	fmt.Printf("database upgraded: target=%s PostgreSQL=%d tables=%d fingerprint=%s\n", *target, verification.ServerVersion, len(verification.Tables), verification.CatalogFingerprint)
	return nil
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
