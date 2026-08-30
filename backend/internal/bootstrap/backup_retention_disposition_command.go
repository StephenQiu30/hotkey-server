package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

func runBackupRetentionDispositionCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	manifestPath, err := parseBackupRetentionDispositionFlags(args)
	if err != nil {
		return err
	}
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate backup retention disposition configuration: %w", err)
	}
	info, err := os.Lstat(manifestPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > 64*1024 {
		return errors.New("backup retention disposition manifest must be a bounded regular file")
	}
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		return errors.New("read backup retention disposition manifest")
	}
	runtime, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	service, err := operationsapplication.NewBackupRetentionDispositionService(
		operationspostgres.NewBackupRetentionDispositionRepository(runtime),
	)
	if err != nil {
		return err
	}
	receipt, err := service.Record(ctx, payload)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "backup retention disposition recorded: disposition_id=%d backup_run_sha256=%s status=%s\n",
		receipt.DispositionID, receipt.BackupRunSHA256, receipt.Status)
	return nil
}

func parseBackupRetentionDispositionFlags(args []string) (string, error) {
	flags := flag.NewFlagSet("hotkey maintenance record-backup-retention-disposition", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	manifest := flags.String("manifest", "", "immutable hotkey-backup-retention-disposition-v1 manifest")
	if err := flags.Parse(args); err != nil {
		return "", fmt.Errorf("parse backup retention disposition flags: %w", err)
	}
	if flags.NArg() != 0 || strings.TrimSpace(*manifest) == "" {
		return "", errors.New("record-backup-retention-disposition requires exactly one --manifest path")
	}
	clean := filepath.Clean(*manifest)
	if clean == "." || clean == string(filepath.Separator) {
		return "", errors.New("backup retention disposition manifest path is invalid")
	}
	return clean, nil
}
