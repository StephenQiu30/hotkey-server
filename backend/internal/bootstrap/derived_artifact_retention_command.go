package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	ingestionapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/application"
	ingestionpostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/ingestion/infrastructure/postgres"
	knowledgevault "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/vault"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

type derivedArtifactRetentionFlags struct {
	BatchSize     int
	Apply         bool
	ConfirmDelete bool
}

func parseDerivedArtifactRetentionFlags(args []string) (derivedArtifactRetentionFlags, error) {
	flags := flag.NewFlagSet("hotkey maintenance expire-derived-artifacts", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	batchSize := flags.Int("batch-size", 25, "bounded derived artifact deletion batch size")
	apply := flags.Bool("apply", false, "delete approved automatic Vault projections and tombstone their metadata")
	confirmDelete := flags.Bool("confirm-delete", false, "confirm irreversible automatic projection deletion")
	if err := flags.Parse(args); err != nil {
		return derivedArtifactRetentionFlags{}, fmt.Errorf("parse derived artifact retention flags: %w", err)
	}
	command := derivedArtifactRetentionFlags{BatchSize: *batchSize, Apply: *apply, ConfirmDelete: *confirmDelete}
	if len(flags.Args()) != 0 {
		return derivedArtifactRetentionFlags{}, fmt.Errorf("unexpected derived artifact retention arguments: %v", flags.Args())
	}
	if command.BatchSize < 1 || command.BatchSize > ingestionapplication.MaximumDerivedArtifactRetentionBatch ||
		!command.Apply || !command.ConfirmDelete {
		return derivedArtifactRetentionFlags{}, errors.New("derived artifact retention requires a bounded --batch-size, --apply, and --confirm-delete")
	}
	return command, nil
}

func runDerivedArtifactRetentionCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	command, err := parseDerivedArtifactRetentionFlags(args)
	if err != nil {
		return err
	}
	if output == nil {
		return errors.New("derived artifact retention output is required")
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return errors.New("derived artifact retention database URL is required")
	}
	if strings.TrimSpace(cfg.VaultPath) == "" {
		return errors.New("derived artifact retention Vault path is required")
	}
	runtime, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	service, err := ingestionapplication.NewDerivedArtifactRetentionService(ingestionapplication.DerivedArtifactRetentionDependencies{
		Repository: ingestionpostgres.NewDerivedArtifactRetentionRepository(runtime),
		Deleter:    knowledgevault.NewWriter(cfg.VaultPath),
	})
	if err != nil {
		return err
	}
	result, err := service.Run(ctx, ingestionapplication.RunDerivedArtifactRetentionCommand{
		At: time.Now().UTC(), Limit: command.BatchSize,
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "derived artifact retention completed: claimed=%d deleted=%d failed=%d has_more=%t\n",
		result.Claimed, result.Deleted, result.Failed, result.HasMore)
	return nil
}
