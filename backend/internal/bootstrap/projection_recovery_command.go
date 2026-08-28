package bootstrap

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	knowledgeapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/application"
	knowledgeminio "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/minio"
	knowledgepostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/postgres"
	knowledgevault "github.com/StephenQiu30/hotkey-server/backend/internal/modules/knowledge/infrastructure/vault"
	operationsapplication "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/application"
	operationspostgres "github.com/StephenQiu30/hotkey-server/backend/internal/modules/operations/infrastructure/postgres"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/config"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/database"
	"github.com/StephenQiu30/hotkey-server/backend/internal/platform/queue"
)

func runProjectionRecoveryCommand(ctx context.Context, cfg config.Config, args []string, output io.Writer) error {
	command, err := parseProjectionRecoveryFlags(args)
	if err != nil {
		return err
	}
	if err := cfg.ValidateRuntime(); err != nil {
		return fmt.Errorf("validate projection recovery configuration: %w", err)
	}
	// A CLI confirmation cannot disable a live connector. Apply is therefore
	// additionally fenced against production configuration and enabled SMTP;
	// the runbook still requires an isolated network with all other egress
	// denied before the flag may truthfully be supplied.
	if command.Apply && (cfg.Environment == "production" || cfg.Authentication.SMTP.Enabled) {
		return errors.New("projection recovery apply requires a non-production configuration with SMTP disabled")
	}
	runtime, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	snapshots, err := knowledgeminio.NewStore(cfg.MinIO)
	if err != nil {
		return err
	}
	knowledgeRepository := knowledgepostgres.NewRepository(runtime)
	vault := knowledgevault.NewWriter(cfg.VaultPath)
	vaultRecovery := knowledgeapplication.NewVaultRecoveryService(
		knowledgeRepository, vault, snapshots, nil, operationspostgres.NewAuditWriter(runtime),
	)
	repository, err := operationspostgres.NewProjectionRecoveryRepository(runtime, vaultRecovery, queue.NewStore(runtime))
	if err != nil {
		return err
	}
	service, err := operationsapplication.NewProjectionRecoveryService(repository)
	if err != nil {
		return err
	}
	result, err := service.Recover(ctx, command)
	if err != nil {
		return err
	}
	if command.DryRun {
		_, _ = fmt.Fprintf(output,
			"projection recovery dry-run: facts=%s outbox=%d notifications=%d read_receipts=%d delivery_attempts=%d disposable_claims=%d started_claims=%d unknown_attempts=%d vault_missing=%d search_missing=%d manual_regions=%s blockers=%s\n",
			result.Inspection.Facts.FingerprintSHA256,
			result.Inspection.Facts.NotificationOutboxCount, result.Inspection.Facts.UserNotificationCount,
			result.Inspection.Facts.ReadReceiptCount, result.Inspection.Facts.DeliveryAttemptCount,
			result.Inspection.DisposableDeliveryClaimCount, result.Inspection.StartedDeliveryClaimCount,
			result.Inspection.UnknownDeliveryAttemptCount, result.Inspection.MissingVaultProjectionCount,
			result.Inspection.MissingSearchProjectionCount, result.Inspection.VaultManualRegionFingerprintSHA256,
			strings.Join(result.Inspection.Blockers, ","))
		if len(result.Inspection.Blockers) != 0 {
			return fmt.Errorf("projection recovery dry-run found blockers: %s", strings.Join(result.Inspection.Blockers, ","))
		}
		return nil
	}
	_, _ = fmt.Fprintf(output,
		"projection recovery scheduled: run_id=%d run_sha256=%s facts=%s manual_regions=%s claims_removed=%d vault_jobs=%d search_jobs=%d started_claims=%d unknown_attempts=%d differences=%d\n",
		result.Receipt.RunID, result.Receipt.RunSHA256, result.Receipt.AfterFacts.FingerprintSHA256,
		result.Receipt.AfterVaultManualRegionFingerprintSHA256, result.Receipt.RemovedDeliveryClaimCount,
		result.Receipt.ScheduledVaultRecoveryCount, result.Receipt.ScheduledSearchRebuildCount,
		result.Receipt.PreservedStartedClaimCount, result.Receipt.PreservedUnknownAttemptCount,
		len(result.Receipt.Differences))
	return nil
}

func parseProjectionRecoveryFlags(args []string) (operationsapplication.ProjectionRecoveryCommand, error) {
	flags := flag.NewFlagSet("hotkey maintenance recover-projections", flag.ContinueOnError)
	flags.SetOutput(new(discardWriter))
	dryRun := flags.Bool("dry-run", false, "inspect recovery catalog without changing state")
	apply := flags.Bool("apply", false, "clear disposable claims and schedule missing projections")
	confirmIsolated := flags.Bool("confirm-isolated", false, "confirm a restored isolated environment")
	productionEgressDisabled := flags.Bool("production-egress-disabled", false, "confirm production source and notification egress is disabled")
	operatorID := flags.String("operator-id", "", "operator record identity")
	reviewerID := flags.String("reviewer-id", "", "independent reviewer record identity")
	runSHA256 := flags.String("run-sha256", "", "immutable recovery run identity digest")
	backupEvidenceSHA256 := flags.String("backup-evidence-sha256", "", "consistent backup evidence digest")
	rehearsalEvidenceSHA256 := flags.String("rehearsal-evidence-sha256", "", "restored-environment rehearsal evidence digest")
	if err := flags.Parse(args); err != nil {
		return operationsapplication.ProjectionRecoveryCommand{}, fmt.Errorf("parse projection recovery flags: %w", err)
	}
	if flags.NArg() != 0 {
		return operationsapplication.ProjectionRecoveryCommand{}, fmt.Errorf("unexpected projection recovery arguments: %v", flags.Args())
	}
	command := operationsapplication.ProjectionRecoveryCommand{
		DryRun: *dryRun, Apply: *apply, ConfirmIsolated: *confirmIsolated,
		ProductionEgressDisabled: *productionEgressDisabled,
		OperatorID:               *operatorID, ReviewerID: *reviewerID, RunSHA256: *runSHA256,
		BackupEvidenceSHA256: *backupEvidenceSHA256, RehearsalEvidenceSHA256: *rehearsalEvidenceSHA256,
	}
	if command.DryRun == command.Apply {
		return operationsapplication.ProjectionRecoveryCommand{}, errors.New("projection recovery requires exactly one of --dry-run or --apply")
	}
	if command.DryRun {
		if command.ConfirmIsolated || command.ProductionEgressDisabled || command.OperatorID != "" || command.ReviewerID != "" ||
			command.RunSHA256 != "" || command.BackupEvidenceSHA256 != "" || command.RehearsalEvidenceSHA256 != "" {
			return operationsapplication.ProjectionRecoveryCommand{}, errors.New("projection recovery dry-run does not accept mutation evidence")
		}
		return command, nil
	}
	if !command.ConfirmIsolated || !command.ProductionEgressDisabled || command.OperatorID == "" || command.ReviewerID == "" ||
		command.OperatorID == command.ReviewerID || !maintenanceFlagSHA256(command.RunSHA256) ||
		!maintenanceFlagSHA256(command.BackupEvidenceSHA256) || !maintenanceFlagSHA256(command.RehearsalEvidenceSHA256) {
		return operationsapplication.ProjectionRecoveryCommand{}, errors.New("projection recovery apply requires isolated/egress confirmation, independent operator/reviewer, and run/backup/rehearsal digests")
	}
	return command, nil
}
