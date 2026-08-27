package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	sourceapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/application"
	sourceminio "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/minio"
	sourcepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

type rawEvidenceRetentionFlags struct {
	BatchSize     int
	Apply         bool
	ConfirmDelete bool
}

func parseRawEvidenceRetentionFlags(args []string) (rawEvidenceRetentionFlags, error) {
	flags := flag.NewFlagSet("hotkey maintenance expire-raw-evidence", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	batchSize := flags.Int("batch-size", 25, "bounded raw evidence deletion batch size")
	apply := flags.Bool("apply", false, "delete expired raw evidence objects and tombstone their metadata")
	confirmDelete := flags.Bool("confirm-delete", false, "confirm irreversible object deletion under approved retention policy")
	if err := flags.Parse(args); err != nil {
		return rawEvidenceRetentionFlags{}, fmt.Errorf("parse raw evidence retention flags: %w", err)
	}
	command := rawEvidenceRetentionFlags{BatchSize: *batchSize, Apply: *apply, ConfirmDelete: *confirmDelete}
	if len(flags.Args()) != 0 {
		return rawEvidenceRetentionFlags{}, fmt.Errorf("unexpected raw evidence retention arguments: %v", flags.Args())
	}
	if command.BatchSize < 1 || command.BatchSize > sourceapplication.MaximumRawEvidenceRetentionBatch || !command.Apply || !command.ConfirmDelete {
		return rawEvidenceRetentionFlags{}, errors.New("raw evidence retention requires a bounded --batch-size, --apply, and --confirm-delete")
	}
	return command, nil
}

func runRawEvidenceRetentionCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	command, err := parseRawEvidenceRetentionFlags(args)
	if err != nil {
		return err
	}
	if output == nil {
		return errors.New("raw evidence retention output is required")
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return errors.New("raw evidence retention database URL is required")
	}
	if err := cfg.MinIO.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate raw evidence retention MinIO configuration: %w", err)
	}
	runtime, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	store, err := sourceminio.NewRawEvidenceStore(cfg.MinIO)
	if err != nil {
		return err
	}
	service, err := sourceapplication.NewRawEvidenceRetentionService(sourceapplication.RawEvidenceRetentionDependencies{
		Repository: sourcepostgres.NewRawEvidenceRetentionRepository(runtime), Deleter: store,
	})
	if err != nil {
		return err
	}
	result, err := service.Run(ctx, sourceapplication.RunRawEvidenceRetentionCommand{At: time.Now().UTC(), Limit: command.BatchSize})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "raw evidence retention completed: claimed=%d deleted=%d failed=%d has_more=%t\n",
		result.Claimed, result.Deleted, result.Failed, result.HasMore)
	return nil
}
