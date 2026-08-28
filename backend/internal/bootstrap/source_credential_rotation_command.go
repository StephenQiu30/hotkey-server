package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	operationsdomain "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/domain"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	sourcecredentialstore "github.com/StephenQiu30/hotkey-server/backend/internal/modules/source/infrastructure/credentialstore"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
)

var errSourceCredentialRotationDryRun = errors.New("source credential rotation dry-run rollback")

type sourceCredentialRotationCommand struct {
	BatchSize   int
	ActorUserID int64
	DryRun      bool
	Apply       bool
}

func runSourceCredentialRotationCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	command, err := parseSourceCredentialRotationFlags(args)
	if err != nil {
		return err
	}
	if output == nil {
		return errors.New("source credential rotation output is required")
	}
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate source credential rotation configuration: %w", err)
	}
	if strings.TrimSpace(cfg.SourceCredentialMasterKey) == "" || strings.TrimSpace(cfg.SourceCredentialPreviousMasterKey) == "" {
		return errors.New("source credential rotation requires current and previous master keys")
	}
	runtime, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	store, err := sourcecredentialstore.NewStoreWithKeyring(runtime, cfg.SourceCredentialMasterKeyVersion, cfg.SourceCredentialMasterKey, map[int]string{
		cfg.SourceCredentialPreviousMasterKeyVersion: cfg.SourceCredentialPreviousMasterKey,
	})
	if err != nil {
		return err
	}
	var result sourcecredentialstore.RotationBatchResult
	if command.DryRun {
		err = runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
			result, err = store.RotateBatch(transactionCtx, command.ActorUserID, command.BatchSize)
			if err != nil {
				return err
			}
			return errSourceCredentialRotationDryRun
		})
		if !errors.Is(err, errSourceCredentialRotationDryRun) {
			return err
		}
		_, _ = fmt.Fprintf(output, "source credential rotation dry-run: current_key_version=%d scanned=%d would_rotate=%d remaining=%d\n", result.CurrentVersion, result.Scanned, result.Rotated, result.Remaining)
		return nil
	}

	audit := operationspostgres.NewAuditWriter(runtime)
	err = runtime.WithinTransaction(ctx, func(transactionCtx context.Context, _ database.Transaction) error {
		result, err = store.RotateBatch(transactionCtx, command.ActorUserID, command.BatchSize)
		if err != nil {
			return err
		}
		return audit.Write(transactionCtx, operationsdomain.AuditEntry{
			ActorType: "user", ActorID: command.ActorUserID,
			Action: operationsdomain.ActionSourceCredentialsRotated, ResourceType: "source_credential_keyring",
			Before: map[string]any{"key_version": cfg.SourceCredentialPreviousMasterKeyVersion},
			After: map[string]any{
				"key_version": cfg.SourceCredentialMasterKeyVersion, "rotated_count": result.Rotated, "remaining_count": result.Remaining,
			},
			Result: operationsdomain.AuditResultSuccess,
		})
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(output, "source credential rotation completed: current_key_version=%d scanned=%d rotated=%d remaining=%d\n", result.CurrentVersion, result.Scanned, result.Rotated, result.Remaining)
	return nil
}

func parseSourceCredentialRotationFlags(args []string) (sourceCredentialRotationCommand, error) {
	flags := flag.NewFlagSet("hotkey maintenance rotate-source-credentials", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	batchSize := flags.Int("batch-size", 100, "bounded credential rotation batch size")
	actorUserID := flags.Int64("actor-user-id", 0, "admin user ID recorded on updated credentials and audit")
	dryRun := flags.Bool("dry-run", false, "preflight one batch and roll it back")
	apply := flags.Bool("apply", false, "rotate one bounded batch")
	if err := flags.Parse(args); err != nil {
		return sourceCredentialRotationCommand{}, fmt.Errorf("parse source credential rotation flags: %w", err)
	}
	if flags.NArg() != 0 || *batchSize < 1 || *batchSize > 1000 || *actorUserID <= 0 || *dryRun == *apply {
		return sourceCredentialRotationCommand{}, errors.New("source credential rotation requires --actor-user-id, a batch size from 1 to 1000, and exactly one of --dry-run or --apply")
	}
	return sourceCredentialRotationCommand{BatchSize: *batchSize, ActorUserID: *actorUserID, DryRun: *dryRun, Apply: *apply}, nil
}
